"""
Executor module for Agent - handles route execution and management.

This module implements routing table reading, route command generation,
and route application logic for the SD-WAN system.
"""

import subprocess
import logging
import ipaddress
from typing import Dict, List, Optional, Tuple

from models import RouteConfig


logger = logging.getLogger(__name__)


class Executor:
    """
    Route executor for managing Linux kernel routing table.
    
    Handles reading current routes, generating route commands, and applying
    route changes to the kernel routing table via iproute2 commands.
    
    Attributes:
        wg_interface: WireGuard interface name (default: "wg0")
        allowed_subnet: Subnet that routes are allowed to modify
    """
    
    def __init__(self, wg_interface: str = "wg0", allowed_subnet: str = "10.254.0.0/24"):
        """
        Initialize Executor.
        
        Args:
            wg_interface: WireGuard interface name (default: "wg0")
            allowed_subnet: CIDR subnet for safety constraint (default: "10.254.0.0/24")
        """
        self.wg_interface = wg_interface
        self.allowed_subnet = ipaddress.ip_network(allowed_subnet)
        self.current_routes: Dict[str, str] = {}
        
        logger.info(f"Executor initialized: interface={wg_interface}, "
                   f"allowed_subnet={allowed_subnet}")
    
    def get_current_routes(self) -> Dict[str, str]:
        """
        Read current routing table from kernel.
        
        Executes 'ip route show table main' and parses the output to extract
        routes for the WireGuard interface.
        
        Returns:
            Dictionary mapping destination IP to next hop IP
            Example: {"10.254.0.3": "10.254.0.2", "10.254.0.4": "direct"}
        
        Raises:
            subprocess.CalledProcessError: If ip route command fails
        """
        try:
            # Execute ip route show table main
            result = subprocess.run(
                ["ip", "route", "show", "table", "main"],
                capture_output=True,
                text=True,
                check=True,
                timeout=5
            )
            
            routes = {}
            
            # Parse output line by line
            for line in result.stdout.splitlines():
                line = line.strip()
                if not line:
                    continue
                
                # Look for routes on our WireGuard interface
                if self.wg_interface not in line:
                    continue
                
                # Parse different route formats:
                # 1. "10.254.0.3 via 10.254.0.2 dev wg0" (relay route)
                # 2. "10.254.0.3 dev wg0" (direct route)
                parts = line.split()
                
                if len(parts) < 3:
                    continue
                
                # First part should be the destination
                dst = parts[0]
                
                # Check if this is a host route (/32)
                if '/' in dst:
                    # Already has CIDR notation
                    if not dst.endswith('/32'):
                        continue  # Skip non-host routes
                    dst_ip = dst.split('/')[0]
                else:
                    # No CIDR notation, assume /32
                    dst_ip = dst
                
                # Check if "via" keyword is present (relay route)
                if "via" in parts:
                    via_index = parts.index("via")
                    if via_index + 1 < len(parts):
                        next_hop = parts[via_index + 1]
                        routes[dst_ip] = next_hop
                else:
                    # Direct route (no via)
                    routes[dst_ip] = "direct"
            
            logger.debug(f"Current routes: {routes}")
            self.current_routes = routes
            return routes
            
        except subprocess.TimeoutExpired:
            logger.error("Timeout executing 'ip route show'")
            return {}
        except subprocess.CalledProcessError as e:
            logger.error(f"Failed to read routing table: {e.stderr}")
            return {}
        except Exception as e:
            logger.error(f"Unexpected error reading routes: {e}", exc_info=True)
            return {}

    def _is_ip_in_allowed_subnet(self, ip_str: str) -> bool:
        """
        Check if an IP address is within the allowed subnet.
        
        Implements subnet safety constraint to prevent modifying routes
        outside the designated WireGuard subnet.
        
        Args:
            ip_str: IP address string to check
        
        Returns:
            True if IP is in allowed subnet, False otherwise
        """
        try:
            ip = ipaddress.ip_address(ip_str)
            return ip in self.allowed_subnet
        except ValueError:
            logger.error(f"Invalid IP address: {ip_str}")
            return False
    
    def generate_route_add_command(self, dst_ip: str, next_hop: str) -> Optional[List[str]]:
        """
        Generate 'ip route replace' command for adding/modifying a relay route.
        
        Args:
            dst_ip: Destination IP address
            next_hop: Next hop IP address (relay node)
        
        Returns:
            Command as list of strings, or None if validation fails
            Example: ["ip", "route", "replace", "10.254.0.3/32", "via", "10.254.0.2", "dev", "wg0"]
        """
        # Subnet safety check
        if not self._is_ip_in_allowed_subnet(dst_ip):
            logger.error(f"Route add rejected: {dst_ip} not in allowed subnet {self.allowed_subnet}")
            return None
        
        if not self._is_ip_in_allowed_subnet(next_hop):
            logger.error(f"Route add rejected: next_hop {next_hop} not in allowed subnet {self.allowed_subnet}")
            return None
        
        # Generate command
        command = [
            "ip", "route", "replace",
            f"{dst_ip}/32",
            "via", next_hop,
            "dev", self.wg_interface
        ]
        
        logger.debug(f"Generated add command: {' '.join(command)}")
        return command
    
    def generate_route_del_command(self, dst_ip: str) -> Optional[List[str]]:
        """
        Generate 'ip route del' command for removing a relay route.
        
        This restores direct routing by removing the specific host route,
        allowing traffic to fall back to WireGuard's default routing.
        
        Args:
            dst_ip: Destination IP address
        
        Returns:
            Command as list of strings, or None if validation fails
            Example: ["ip", "route", "del", "10.254.0.3/32", "dev", "wg0"]
        """
        # Subnet safety check
        if not self._is_ip_in_allowed_subnet(dst_ip):
            logger.error(f"Route del rejected: {dst_ip} not in allowed subnet {self.allowed_subnet}")
            return None
        
        # Generate command
        command = [
            "ip", "route", "del",
            f"{dst_ip}/32",
            "dev", self.wg_interface
        ]
        
        logger.debug(f"Generated del command: {' '.join(command)}")
        return command

    def calculate_route_diff(
        self,
        desired_routes: Dict[str, str],
        current_routes: Dict[str, str]
    ) -> Tuple[Dict[str, str], Dict[str, str], List[str]]:
        """
        Calculate differences between desired and current routing tables.
        
        Compares two routing configurations and determines which routes need
        to be added, modified, or deleted.
        
        Args:
            desired_routes: Target routing configuration {dst_ip: next_hop}
            current_routes: Current routing configuration {dst_ip: next_hop}
        
        Returns:
            Tuple of (routes_to_add, routes_to_modify, routes_to_delete)
            - routes_to_add: New routes not in current table
            - routes_to_modify: Routes with different next_hop
            - routes_to_delete: Routes in current but not in desired
        """
        routes_to_add = {}
        routes_to_modify = {}
        routes_to_delete = []
        
        # Find routes to add or modify
        for dst_ip, next_hop in desired_routes.items():
            if dst_ip not in current_routes:
                # New route
                routes_to_add[dst_ip] = next_hop
            elif current_routes[dst_ip] != next_hop:
                # Route exists but next_hop changed
                routes_to_modify[dst_ip] = next_hop
        
        # Find routes to delete
        for dst_ip in current_routes:
            if dst_ip not in desired_routes:
                routes_to_delete.append(dst_ip)
        
        logger.debug(f"Route diff: add={len(routes_to_add)}, "
                    f"modify={len(routes_to_modify)}, delete={len(routes_to_delete)}")
        
        return routes_to_add, routes_to_modify, routes_to_delete
    
    def apply_route(self, dst_ip: str, next_hop: str) -> bool:
        """
        Apply a single route change to the kernel routing table.
        
        Args:
            dst_ip: Destination IP address
            next_hop: Next hop IP address, or "direct" for direct routing
        
        Returns:
            True if route was applied successfully, False otherwise
        """
        try:
            if next_hop == "direct":
                # Remove specific route to restore direct routing
                command = self.generate_route_del_command(dst_ip)
                if command is None:
                    return False
                
                # Execute delete command (may fail if route doesn't exist)
                result = subprocess.run(
                    command,
                    capture_output=True,
                    text=True,
                    timeout=5
                )
                
                if result.returncode != 0:
                    # Route might not exist, which is okay for delete
                    logger.debug(f"Route delete returned {result.returncode}: {result.stderr}")
                
                logger.info(f"Applied direct route to {dst_ip}")
                return True
            else:
                # Add or replace relay route
                command = self.generate_route_add_command(dst_ip, next_hop)
                if command is None:
                    return False
                
                # Execute replace command
                result = subprocess.run(
                    command,
                    capture_output=True,
                    text=True,
                    check=True,
                    timeout=5
                )
                
                logger.info(f"Applied relay route: {dst_ip} via {next_hop}")
                return True
                
        except subprocess.TimeoutExpired:
            logger.error(f"Timeout applying route for {dst_ip}")
            return False
        except subprocess.CalledProcessError as e:
            logger.error(f"Failed to apply route for {dst_ip}: {e.stderr}")
            return False
        except Exception as e:
            logger.error(f"Unexpected error applying route for {dst_ip}: {e}", exc_info=True)
            return False
    
    def flush_all_routes(self) -> bool:
        """
        Flush all dynamically added routes for the WireGuard interface.
        
        This is used in fallback mode to remove all relay routes and
        restore WireGuard's default Full Mesh routing.
        
        Returns:
            True if routes were flushed successfully, False otherwise
        """
        try:
            # Get current routes to know what to flush
            current_routes = self.get_current_routes()
            
            if not current_routes:
                logger.info("No routes to flush")
                return True
            
            # Delete each route
            success = True
            for dst_ip in current_routes.keys():
                if not self.apply_route(dst_ip, "direct"):
                    logger.error(f"Failed to flush route for {dst_ip}")
                    success = False
            
            if success:
                logger.info(f"Flushed {len(current_routes)} routes successfully")
            else:
                logger.warning("Route flush completed with errors")
            
            return success
            
        except Exception as e:
            logger.error(f"Unexpected error flushing routes: {e}", exc_info=True)
            return False
        """
        Synchronize kernel routing table with desired configuration.
        
        Compares desired routes with current routes and applies necessary changes.
        Implements error handling and retry logic.
        
        Args:
            desired_routes: List of RouteConfig objects from Controller
        
        Returns:
            True if all routes were applied successfully, False if any failed
        """
        # Convert RouteConfig list to dict format
        desired_dict = {}
        for route in desired_routes:
            # Extract IP from CIDR notation
            dst_ip = route.dst_cidr.split('/')[0]
            desired_dict[dst_ip] = route.next_hop
        
        # Get current routes
        current_dict = self.get_current_routes()
        
        # Calculate diff
        to_add, to_modify, to_delete = self.calculate_route_diff(desired_dict, current_dict)
        
        # Apply changes
        success = True
        
        # Apply additions
        for dst_ip, next_hop in to_add.items():
            if not self.apply_route(dst_ip, next_hop):
                logger.error(f"Failed to add route for {dst_ip}")
                success = False
        
        # Apply modifications (same as additions with 'replace')
        for dst_ip, next_hop in to_modify.items():
            if not self.apply_route(dst_ip, next_hop):
                logger.error(f"Failed to modify route for {dst_ip}")
                success = False
        
        # Apply deletions
        for dst_ip in to_delete:
            if not self.apply_route(dst_ip, "direct"):
                logger.error(f"Failed to delete route for {dst_ip}")
                success = False
        
        if success:
            logger.info(f"Route sync complete: {len(to_add)} added, "
                       f"{len(to_modify)} modified, {len(to_delete)} deleted")
        else:
            logger.warning("Route sync completed with errors")
        
        return success

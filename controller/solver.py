"""
Route Solver for Controller.

This module implements the path computation engine that calculates optimal
routes using Dijkstra's algorithm with hysteresis to prevent route flapping.

Requirements: 4.1, 4.2, 4.3, 4.4, 4.5, 7.1, 9.1
"""

from typing import Dict, List, Optional, Tuple
import networkx as nx
from models import RouteConfig


def calculate_cost(latency_ms: float, loss_rate: float, penalty_factor: int = 100) -> float:
    """
    Calculate link cost based on latency and packet loss.
    
    The cost function combines latency and loss rate to produce a single
    metric for path optimization. Higher loss rates are heavily penalized.
    
    Args:
        latency_ms: Round-trip time in milliseconds
        loss_rate: Packet loss rate as decimal (0.0 = no loss, 1.0 = 100% loss)
        penalty_factor: Multiplier for loss rate penalty (default: 100)
    
    Returns:
        Calculated cost value
    
    Formula:
        Cost = Latency_ms + (Loss_rate × Penalty_Factor)
    
    Requirements: 4.1 - Link cost calculation formula
    
    Examples:
        >>> calculate_cost(50.0, 0.0)
        50.0
        >>> calculate_cost(50.0, 0.01)
        51.0
        >>> calculate_cost(100.0, 0.1)
        110.0
    """
    return latency_ms + (loss_rate * penalty_factor)


class RouteSolver:
    """
    Path computation engine for SD-WAN routing.
    
    This class builds a network graph from topology data and computes optimal
    paths using Dijkstra's algorithm. It implements hysteresis to prevent
    route flapping when network conditions fluctuate.
    
    Attributes:
        penalty_factor: Multiplier for packet loss in cost calculation
        hysteresis: Threshold for route switching (0.15 = 15% improvement required)
        previous_costs: Historical path costs for hysteresis logic
    """
    
    def __init__(self, penalty_factor: int = 100, hysteresis: float = 0.15):
        """
        Initialize the route solver.
        
        Args:
            penalty_factor: Multiplier for loss rate in cost calculation (default: 100)
            hysteresis: Minimum improvement threshold for route switching (default: 0.15)
        
        Requirements: 4.1, 4.5, 7.1
        """
        self.penalty_factor = penalty_factor
        self.hysteresis = hysteresis
        self.previous_costs: Dict[Tuple[str, str], float] = {}
    
    def build_graph(self, topology_data: Dict[str, Dict]) -> nx.DiGraph:
        """
        Build a directed graph from topology database data.
        
        Creates a NetworkX DiGraph where nodes represent agents and edges
        represent links with weights calculated from RTT and loss rate.
        
        Args:
            topology_data: Dictionary mapping agent_id to telemetry data
                          Format: {agent_id: {"timestamp": int, "metrics": {...}}}
        
        Returns:
            NetworkX directed graph with weighted edges
        
        Requirements: 4.2 - Graph construction from telemetry
        
        Graph Structure:
            - Nodes: agent IDs (e.g., "10.254.0.1")
            - Edges: directed links with 'weight' attribute (cost)
            - Unreachable links: weight = float('inf')
        """
        G = nx.DiGraph()
        
        # Add all nodes
        for agent_id in topology_data.keys():
            G.add_node(agent_id)
        
        # Add edges with costs
        for source, data in topology_data.items():
            metrics = data.get("metrics", {})
            
            for target, metric_data in metrics.items():
                rtt = metric_data.get("rtt")
                loss = metric_data.get("loss", 0.0)
                
                if rtt is None:  # Link is down
                    cost = float('inf')
                else:
                    cost = calculate_cost(rtt, loss, self.penalty_factor)
                
                G.add_edge(source, target, weight=cost)
        
        return G
    
    def compute_routes_for(self, source_agent: str, topology_data: Dict[str, Dict]) -> List[RouteConfig]:
        """
        Compute optimal routes for a specific agent.
        
        Uses Dijkstra's algorithm to find shortest paths from the source agent
        to all other nodes. Applies hysteresis to prevent route flapping.
        
        Args:
            source_agent: Agent ID requesting routes
            topology_data: Complete topology database data
        
        Returns:
            List of RouteConfig objects specifying next hops
        
        Requirements: 4.3, 4.4, 4.5, 7.1 - Dijkstra algorithm, next hop computation, hysteresis
        
        Route Selection Logic:
            1. Compute shortest path to each destination
            2. Extract next hop (second node in path, or "direct" if adjacent)
            3. Apply hysteresis: only switch if new_cost < old_cost × (1 - threshold)
            4. Generate RouteConfig for each route change
        """
        G = self.build_graph(topology_data)
        routes = []
        
        # Check if source agent exists in graph
        if source_agent not in G:
            return routes
        
        for target in G.nodes():
            if target == source_agent:
                continue
            
            try:
                # Compute shortest path
                path = nx.shortest_path(G, source_agent, target, weight='weight')
                new_cost = nx.shortest_path_length(G, source_agent, target, weight='weight')
                
                # Skip if cost is infinite (unreachable)
                if new_cost == float('inf'):
                    continue
                
                # Apply hysteresis
                cost_key = (source_agent, target)
                old_cost = self.previous_costs.get(cost_key, float('inf'))
                
                # Only switch if new cost is significantly better
                if new_cost < old_cost * (1 - self.hysteresis):
                    # Update cost history
                    self.previous_costs[cost_key] = new_cost
                    
                    # Determine next hop
                    if len(path) == 1:
                        # Source equals target (shouldn't happen, but handle it)
                        continue
                    elif len(path) == 2:
                        # Direct connection
                        next_hop = "direct"
                        reason = "default"
                    else:
                        # Multi-hop path, use second node as next hop
                        next_hop = path[1]
                        reason = "optimized_path"
                    
                    routes.append(RouteConfig(
                        dst_cidr=f"{target}/32",
                        next_hop=next_hop,
                        reason=reason
                    ))
                
            except nx.NetworkXNoPath:
                # No path available, skip this destination
                continue
            except nx.NodeNotFound:
                # Node not in graph, skip
                continue
        
        return routes
    
    def reset_history(self) -> None:
        """
        Reset the cost history.
        
        Useful for testing or when network topology changes significantly.
        """
        self.previous_costs.clear()
    
    def get_cost_history(self) -> Dict[Tuple[str, str], float]:
        """
        Get a copy of the cost history.
        
        Returns:
            Dictionary mapping (source, target) tuples to their last known costs
        """
        return dict(self.previous_costs)


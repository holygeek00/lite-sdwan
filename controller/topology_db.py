"""
Topology Database for Controller.

This module implements an in-memory database that stores network topology data
received from agents. It maintains a mapping of agent_id to their latest metrics.

Requirements: 3.5
"""

from typing import Dict, Optional, List
from threading import Lock
import time


class TopologyDatabase:
    """
    In-memory topology database for storing agent telemetry data.
    
    This database stores the latest metrics from each agent, maintaining a
    global view of the network topology. Thread-safe for concurrent access.
    
    Attributes:
        _data: Dictionary mapping agent_id to their telemetry data
        _lock: Thread lock for safe concurrent access
    """
    
    def __init__(self):
        """Initialize an empty topology database."""
        self._data: Dict[str, Dict] = {}
        self._lock = Lock()
    
    def store_telemetry(self, agent_id: str, timestamp: int, metrics: Dict[str, Dict]) -> None:
        """
        Store telemetry data from an agent.
        
        Args:
            agent_id: Unique identifier for the agent
            timestamp: Unix timestamp when measurements were collected
            metrics: Dictionary mapping target_ip to {"rtt": float, "loss": float}
        
        Requirements: 3.5 - Store received telemetry in topology database
        """
        with self._lock:
            self._data[agent_id] = {
                "timestamp": timestamp,
                "metrics": metrics
            }
    
    def get_agent_data(self, agent_id: str) -> Optional[Dict]:
        """
        Retrieve telemetry data for a specific agent.
        
        Args:
            agent_id: Unique identifier for the agent
        
        Returns:
            Dictionary containing timestamp and metrics, or None if agent not found
        
        Requirements: 3.5 - Query interface for topology data
        """
        with self._lock:
            return self._data.get(agent_id)
    
    def get_all_agents(self) -> List[str]:
        """
        Get list of all agent IDs in the database.
        
        Returns:
            List of agent identifiers
        
        Requirements: 3.5 - Query interface for topology data
        """
        with self._lock:
            return list(self._data.keys())
    
    def get_all_data(self) -> Dict[str, Dict]:
        """
        Get a copy of all topology data.
        
        Returns:
            Dictionary mapping agent_id to their telemetry data
        
        Requirements: 3.5 - Query interface for topology data
        """
        with self._lock:
            # Return a deep copy to prevent external modifications
            return {
                agent_id: {
                    "timestamp": data["timestamp"],
                    "metrics": {
                        target_ip: dict(metric_data)
                        for target_ip, metric_data in data["metrics"].items()
                    }
                }
                for agent_id, data in self._data.items()
            }
    
    def agent_exists(self, agent_id: str) -> bool:
        """
        Check if an agent exists in the database.
        
        Args:
            agent_id: Unique identifier for the agent
        
        Returns:
            True if agent has sent telemetry, False otherwise
        
        Requirements: 3.5 - Query interface for topology data
        """
        with self._lock:
            return agent_id in self._data
    
    def clear(self) -> None:
        """
        Clear all data from the database.
        
        Useful for testing or resetting the system state.
        """
        with self._lock:
            self._data.clear()
    
    def get_agent_count(self) -> int:
        """
        Get the number of agents in the database.
        
        Returns:
            Count of agents that have sent telemetry
        """
        with self._lock:
            return len(self._data)
    
    def remove_stale_agents(self, max_age_seconds: int = 60) -> List[str]:
        """
        Remove agents whose data is older than max_age_seconds.
        
        Args:
            max_age_seconds: Maximum age of data to keep (default: 60 seconds)
        
        Returns:
            List of removed agent IDs
        
        Note: This is useful for cleaning up agents that have stopped reporting.
        """
        current_time = int(time.time())
        removed = []
        
        with self._lock:
            for agent_id, data in list(self._data.items()):
                if current_time - data["timestamp"] > max_age_seconds:
                    del self._data[agent_id]
                    removed.append(agent_id)
        
        return removed

"""
Unit tests for Topology Database.

Tests the in-memory topology database implementation for storing and
querying agent telemetry data.
"""

import pytest
import time
from controller.topology_db import TopologyDatabase


def test_store_and_retrieve_telemetry():
    """Test storing and retrieving telemetry data."""
    db = TopologyDatabase()
    
    # Store telemetry
    agent_id = "node_shanghai"
    timestamp = 1703830000
    metrics = {
        "10.254.0.2": {"rtt": 35.5, "loss": 0.0},
        "10.254.0.3": {"rtt": 150.2, "loss": 0.05}
    }
    
    db.store_telemetry(agent_id, timestamp, metrics)
    
    # Retrieve and verify
    data = db.get_agent_data(agent_id)
    assert data is not None
    assert data["timestamp"] == timestamp
    assert data["metrics"] == metrics


def test_get_nonexistent_agent():
    """Test retrieving data for an agent that doesn't exist."""
    db = TopologyDatabase()
    
    data = db.get_agent_data("nonexistent_agent")
    assert data is None


def test_agent_exists():
    """Test checking if an agent exists in the database."""
    db = TopologyDatabase()
    
    assert not db.agent_exists("node_shanghai")
    
    db.store_telemetry("node_shanghai", 1703830000, {})
    
    assert db.agent_exists("node_shanghai")


def test_get_all_agents():
    """Test retrieving list of all agents."""
    db = TopologyDatabase()
    
    # Initially empty
    assert db.get_all_agents() == []
    
    # Add agents
    db.store_telemetry("node_shanghai", 1703830000, {})
    db.store_telemetry("node_beijing", 1703830005, {})
    
    agents = db.get_all_agents()
    assert len(agents) == 2
    assert "node_shanghai" in agents
    assert "node_beijing" in agents


def test_get_all_data():
    """Test retrieving all topology data."""
    db = TopologyDatabase()
    
    # Store multiple agents
    db.store_telemetry("node_shanghai", 1703830000, {
        "10.254.0.2": {"rtt": 35.5, "loss": 0.0}
    })
    db.store_telemetry("node_beijing", 1703830005, {
        "10.254.0.1": {"rtt": 36.0, "loss": 0.0}
    })
    
    all_data = db.get_all_data()
    assert len(all_data) == 2
    assert "node_shanghai" in all_data
    assert "node_beijing" in all_data
    assert all_data["node_shanghai"]["timestamp"] == 1703830000


def test_update_existing_agent():
    """Test updating telemetry for an existing agent."""
    db = TopologyDatabase()
    
    # Store initial data
    db.store_telemetry("node_shanghai", 1703830000, {
        "10.254.0.2": {"rtt": 35.5, "loss": 0.0}
    })
    
    # Update with new data
    db.store_telemetry("node_shanghai", 1703830010, {
        "10.254.0.2": {"rtt": 40.0, "loss": 0.01}
    })
    
    # Verify updated data
    data = db.get_agent_data("node_shanghai")
    assert data["timestamp"] == 1703830010
    assert data["metrics"]["10.254.0.2"]["rtt"] == 40.0


def test_clear_database():
    """Test clearing all data from the database."""
    db = TopologyDatabase()
    
    # Add data
    db.store_telemetry("node_shanghai", 1703830000, {})
    db.store_telemetry("node_beijing", 1703830005, {})
    
    assert db.get_agent_count() == 2
    
    # Clear
    db.clear()
    
    assert db.get_agent_count() == 0
    assert db.get_all_agents() == []


def test_get_agent_count():
    """Test counting agents in the database."""
    db = TopologyDatabase()
    
    assert db.get_agent_count() == 0
    
    db.store_telemetry("node_shanghai", 1703830000, {})
    assert db.get_agent_count() == 1
    
    db.store_telemetry("node_beijing", 1703830005, {})
    assert db.get_agent_count() == 2


def test_remove_stale_agents():
    """Test removing agents with old data."""
    db = TopologyDatabase()
    
    current_time = int(time.time())
    
    # Add fresh agent
    db.store_telemetry("node_fresh", current_time, {})
    
    # Add stale agent (70 seconds old)
    db.store_telemetry("node_stale", current_time - 70, {})
    
    # Remove stale agents (older than 60 seconds)
    removed = db.remove_stale_agents(max_age_seconds=60)
    
    assert "node_stale" in removed
    assert db.agent_exists("node_fresh")
    assert not db.agent_exists("node_stale")


def test_data_isolation():
    """Test that returned data is isolated from internal storage."""
    db = TopologyDatabase()
    
    metrics = {"10.254.0.2": {"rtt": 35.5, "loss": 0.0}}
    db.store_telemetry("node_shanghai", 1703830000, metrics)
    
    # Get all data and modify it
    all_data = db.get_all_data()
    all_data["node_shanghai"]["metrics"]["10.254.0.2"]["rtt"] = 999.9
    
    # Verify original data is unchanged
    data = db.get_agent_data("node_shanghai")
    assert data["metrics"]["10.254.0.2"]["rtt"] == 35.5


def test_concurrent_access():
    """Test thread-safe concurrent access to the database."""
    import threading
    
    db = TopologyDatabase()
    errors = []
    
    def store_data(agent_id, count):
        try:
            for i in range(count):
                db.store_telemetry(f"{agent_id}_{i}", 1703830000 + i, {})
        except Exception as e:
            errors.append(e)
    
    # Create multiple threads
    threads = []
    for i in range(5):
        t = threading.Thread(target=store_data, args=(f"agent_{i}", 10))
        threads.append(t)
        t.start()
    
    # Wait for all threads
    for t in threads:
        t.join()
    
    # Verify no errors and correct count
    assert len(errors) == 0
    assert db.get_agent_count() == 50  # 5 threads * 10 agents each

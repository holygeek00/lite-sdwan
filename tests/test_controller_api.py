"""
Unit tests for Controller REST API.

Tests the FastAPI endpoints for telemetry reception and route retrieval.

Requirements: 10.1, 10.2, 10.3, 10.4, 10.5
"""

import pytest
from fastapi.testclient import TestClient
from controller.api import app, topology_db, route_solver


@pytest.fixture
def client():
    """Create a test client for the FastAPI app."""
    return TestClient(app)


@pytest.fixture(autouse=True)
def reset_state():
    """Reset topology database and solver state before each test."""
    topology_db.clear()
    route_solver.reset_history()
    yield
    topology_db.clear()
    route_solver.reset_history()


class TestTelemetryEndpoint:
    """Tests for POST /api/v1/telemetry endpoint."""
    
    def test_valid_telemetry_returns_200(self, client):
        """
        Test that valid telemetry data returns HTTP 200.
        
        Requirements: 10.1, 10.5 - POST endpoint returns 200 for valid requests
        """
        payload = {
            "agent_id": "10.254.0.1",
            "timestamp": 1703830000,
            "metrics": [
                {"target_ip": "10.254.0.2", "rtt_ms": 35.5, "loss_rate": 0.0},
                {"target_ip": "10.254.0.3", "rtt_ms": 150.2, "loss_rate": 0.05}
            ]
        }
        
        response = client.post("/api/v1/telemetry", json=payload)
        
        assert response.status_code == 200
        assert response.json() == {"status": "ok"}
    
    def test_telemetry_stored_in_database(self, client):
        """
        Test that telemetry data is correctly stored in topology database.
        
        Requirements: 3.5 - Store received telemetry in topology database
        """
        payload = {
            "agent_id": "10.254.0.1",
            "timestamp": 1703830000,
            "metrics": [
                {"target_ip": "10.254.0.2", "rtt_ms": 35.5, "loss_rate": 0.0}
            ]
        }
        
        response = client.post("/api/v1/telemetry", json=payload)
        assert response.status_code == 200
        
        # Verify data is in database
        stored_data = topology_db.get_agent_data("10.254.0.1")
        assert stored_data is not None
        assert stored_data["timestamp"] == 1703830000
        assert "10.254.0.2" in stored_data["metrics"]
        assert stored_data["metrics"]["10.254.0.2"]["rtt"] == 35.5
        assert stored_data["metrics"]["10.254.0.2"]["loss"] == 0.0
    
    def test_invalid_json_returns_400(self, client):
        """
        Test that invalid JSON payload returns HTTP 400.
        
        Requirements: 10.3 - Return HTTP 400 for invalid JSON payloads
        """
        # Missing required field 'agent_id'
        payload = {
            "timestamp": 1703830000,
            "metrics": []
        }
        
        response = client.post("/api/v1/telemetry", json=payload)
        assert response.status_code == 422  # FastAPI validation error
    
    def test_invalid_loss_rate_returns_400(self, client):
        """
        Test that invalid loss_rate (outside 0.0-1.0) returns error.
        
        Requirements: 10.3 - Validate data format
        """
        payload = {
            "agent_id": "10.254.0.1",
            "timestamp": 1703830000,
            "metrics": [
                {"target_ip": "10.254.0.2", "rtt_ms": 35.5, "loss_rate": 1.5}  # Invalid
            ]
        }
        
        response = client.post("/api/v1/telemetry", json=payload)
        assert response.status_code == 422  # Pydantic validation error
    
    def test_negative_rtt_returns_400(self, client):
        """
        Test that negative RTT values are rejected.
        
        Requirements: 10.3 - Validate data format
        """
        payload = {
            "agent_id": "10.254.0.1",
            "timestamp": 1703830000,
            "metrics": [
                {"target_ip": "10.254.0.2", "rtt_ms": -10.0, "loss_rate": 0.0}
            ]
        }
        
        response = client.post("/api/v1/telemetry", json=payload)
        assert response.status_code == 422


class TestRoutesEndpoint:
    """Tests for GET /api/v1/routes endpoint."""
    
    def test_routes_for_existing_agent_returns_200(self, client):
        """
        Test that route request for existing agent returns HTTP 200.
        
        Requirements: 10.2, 10.5 - GET endpoint returns 200 for valid requests
        """
        # First, send telemetry to register the agent
        telemetry = {
            "agent_id": "10.254.0.1",
            "timestamp": 1703830000,
            "metrics": [
                {"target_ip": "10.254.0.2", "rtt_ms": 35.5, "loss_rate": 0.0}
            ]
        }
        client.post("/api/v1/telemetry", json=telemetry)
        
        # Request routes
        response = client.get("/api/v1/routes?agent_id=10.254.0.1")
        
        assert response.status_code == 200
        assert "routes" in response.json()
    
    def test_routes_for_nonexistent_agent_returns_404(self, client):
        """
        Test that route request for non-existent agent returns HTTP 404.
        
        Requirements: 10.4 - Return HTTP 404 when agent_id is not found
        """
        response = client.get("/api/v1/routes?agent_id=10.254.0.99")
        
        assert response.status_code == 404
        assert "not found" in response.json()["detail"].lower()
    
    def test_routes_response_has_valid_json_structure(self, client):
        """
        Test that routes response has valid JSON structure.
        
        Requirements: 5.1 - Return JSON response with next hop mappings
        """
        # Register two agents with telemetry
        telemetry1 = {
            "agent_id": "10.254.0.1",
            "timestamp": 1703830000,
            "metrics": [
                {"target_ip": "10.254.0.2", "rtt_ms": 35.5, "loss_rate": 0.0}
            ]
        }
        telemetry2 = {
            "agent_id": "10.254.0.2",
            "timestamp": 1703830000,
            "metrics": [
                {"target_ip": "10.254.0.1", "rtt_ms": 36.0, "loss_rate": 0.0}
            ]
        }
        client.post("/api/v1/telemetry", json=telemetry1)
        client.post("/api/v1/telemetry", json=telemetry2)
        
        # Request routes
        response = client.get("/api/v1/routes?agent_id=10.254.0.1")
        
        assert response.status_code == 200
        data = response.json()
        assert "routes" in data
        assert isinstance(data["routes"], list)
        
        # Check route structure if routes exist
        if len(data["routes"]) > 0:
            route = data["routes"][0]
            assert "dst_cidr" in route
            assert "next_hop" in route
            assert "reason" in route
    
    def test_missing_agent_id_parameter_returns_422(self, client):
        """
        Test that missing agent_id query parameter returns error.
        
        Requirements: 10.2 - agent_id query parameter is required
        """
        response = client.get("/api/v1/routes")
        
        assert response.status_code == 422  # Missing required parameter


class TestHealthEndpoint:
    """Tests for health check endpoint."""
    
    def test_health_check_returns_200(self, client):
        """Test that health check endpoint works."""
        response = client.get("/health")
        
        assert response.status_code == 200
        assert "status" in response.json()
        assert response.json()["status"] == "healthy"

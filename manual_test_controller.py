#!/usr/bin/env python3
"""
Manual test script for Controller REST API.

This script demonstrates the Controller API functionality by:
1. Starting the FastAPI server
2. Sending telemetry data from multiple agents
3. Requesting routes for each agent
4. Displaying the computed routes

Usage:
    python manual_test_controller.py
"""

import requests
import time
import json
from threading import Thread
import uvicorn
from controller.api import app


def start_server():
    """Start the FastAPI server in a background thread."""
    uvicorn.run(app, host="127.0.0.1", port=8000, log_level="info")


def test_api():
    """Test the Controller API endpoints."""
    base_url = "http://127.0.0.1:8000"
    
    # Wait for server to start
    print("Waiting for server to start...")
    time.sleep(2)
    
    print("\n" + "="*70)
    print("Testing Controller REST API")
    print("="*70)
    
    # Test 1: Send telemetry from agent 10.254.0.1
    print("\n[Test 1] Sending telemetry from agent 10.254.0.1...")
    telemetry1 = {
        "agent_id": "10.254.0.1",
        "timestamp": int(time.time()),
        "metrics": [
            {"target_ip": "10.254.0.2", "rtt_ms": 35.5, "loss_rate": 0.0},
            {"target_ip": "10.254.0.3", "rtt_ms": 150.2, "loss_rate": 0.05}
        ]
    }
    
    response = requests.post(f"{base_url}/api/v1/telemetry", json=telemetry1)
    print(f"Response: {response.status_code} - {response.json()}")
    
    # Test 2: Send telemetry from agent 10.254.0.2
    print("\n[Test 2] Sending telemetry from agent 10.254.0.2...")
    telemetry2 = {
        "agent_id": "10.254.0.2",
        "timestamp": int(time.time()),
        "metrics": [
            {"target_ip": "10.254.0.1", "rtt_ms": 36.0, "loss_rate": 0.0},
            {"target_ip": "10.254.0.3", "rtt_ms": 80.0, "loss_rate": 0.0}
        ]
    }
    
    response = requests.post(f"{base_url}/api/v1/telemetry", json=telemetry2)
    print(f"Response: {response.status_code} - {response.json()}")
    
    # Test 3: Send telemetry from agent 10.254.0.3
    print("\n[Test 3] Sending telemetry from agent 10.254.0.3...")
    telemetry3 = {
        "agent_id": "10.254.0.3",
        "timestamp": int(time.time()),
        "metrics": [
            {"target_ip": "10.254.0.1", "rtt_ms": 155.0, "loss_rate": 0.05},
            {"target_ip": "10.254.0.2", "rtt_ms": 82.0, "loss_rate": 0.0}
        ]
    }
    
    response = requests.post(f"{base_url}/api/v1/telemetry", json=telemetry3)
    print(f"Response: {response.status_code} - {response.json()}")
    
    # Test 4: Request routes for agent 10.254.0.1
    print("\n[Test 4] Requesting routes for agent 10.254.0.1...")
    response = requests.get(f"{base_url}/api/v1/routes", params={"agent_id": "10.254.0.1"})
    print(f"Response: {response.status_code}")
    routes = response.json()
    print(f"Routes computed: {json.dumps(routes, indent=2)}")
    
    # Test 5: Request routes for agent 10.254.0.3
    print("\n[Test 5] Requesting routes for agent 10.254.0.3...")
    response = requests.get(f"{base_url}/api/v1/routes", params={"agent_id": "10.254.0.3"})
    print(f"Response: {response.status_code}")
    routes = response.json()
    print(f"Routes computed: {json.dumps(routes, indent=2)}")
    
    # Test 6: Request routes for non-existent agent (should return 404)
    print("\n[Test 6] Requesting routes for non-existent agent...")
    response = requests.get(f"{base_url}/api/v1/routes", params={"agent_id": "10.254.0.99"})
    print(f"Response: {response.status_code} - {response.json()}")
    
    # Test 7: Send invalid telemetry (should return 422)
    print("\n[Test 7] Sending invalid telemetry (negative RTT)...")
    invalid_telemetry = {
        "agent_id": "10.254.0.1",
        "timestamp": int(time.time()),
        "metrics": [
            {"target_ip": "10.254.0.2", "rtt_ms": -10.0, "loss_rate": 0.0}
        ]
    }
    
    response = requests.post(f"{base_url}/api/v1/telemetry", json=invalid_telemetry)
    print(f"Response: {response.status_code}")
    
    # Test 8: Health check
    print("\n[Test 8] Health check...")
    response = requests.get(f"{base_url}/health")
    print(f"Response: {response.status_code} - {response.json()}")
    
    print("\n" + "="*70)
    print("All tests completed!")
    print("="*70)
    print("\nPress Ctrl+C to stop the server...")


if __name__ == "__main__":
    # Start server in background thread
    server_thread = Thread(target=start_server, daemon=True)
    server_thread.start()
    
    try:
        # Run tests
        test_api()
        
        # Keep main thread alive
        while True:
            time.sleep(1)
    except KeyboardInterrupt:
        print("\nShutting down...")

"""
Controller REST API for Lite SD-WAN Routing System.

This module implements the FastAPI application that provides endpoints
for agents to send telemetry data and retrieve route configurations.

Requirements: 10.1, 10.2, 10.3, 10.4, 10.5, 3.5, 5.1
"""

from fastapi import FastAPI, HTTPException, Query
from fastapi.responses import JSONResponse
from typing import Optional
import logging

from models import TelemetryRequest, RouteResponse, RouteConfig
from controller.topology_db import TopologyDatabase
from controller.solver import RouteSolver

# Configure logging
logging.basicConfig(
    level=logging.INFO,
    format='%(asctime)s - %(name)s - %(levelname)s - %(message)s'
)
logger = logging.getLogger(__name__)

# Initialize FastAPI app
app = FastAPI(
    title="Lite SD-WAN Controller API",
    description="REST API for SD-WAN routing control plane",
    version="1.0.0"
)

# Initialize global components
topology_db = TopologyDatabase()
route_solver = RouteSolver(penalty_factor=100, hysteresis=0.15)


@app.post("/api/v1/telemetry", status_code=200)
async def receive_telemetry(data: TelemetryRequest) -> JSONResponse:
    """
    Receive telemetry data from an agent.
    
    This endpoint accepts network quality measurements from agents and stores
    them in the topology database for route computation.
    
    Args:
        data: TelemetryRequest containing agent_id, timestamp, and metrics
    
    Returns:
        JSON response with status "ok"
    
    Raises:
        HTTPException 400: If the request data is invalid
    
    Requirements:
        - 10.1: POST endpoint /api/v1/telemetry for receiving probe data
        - 10.3: Return HTTP 400 for invalid JSON payloads
        - 3.5: Store received telemetry in topology database
    
    Example Request:
        POST /api/v1/telemetry
        {
            "agent_id": "10.254.0.1",
            "timestamp": 1703830000,
            "metrics": [
                {"target_ip": "10.254.0.2", "rtt_ms": 35.5, "loss_rate": 0.0},
                {"target_ip": "10.254.0.3", "rtt_ms": 150.2, "loss_rate": 0.05}
            ]
        }
    
    Example Response:
        {"status": "ok"}
    """
    try:
        # Convert metrics list to dictionary format for topology database
        metrics_dict = {
            metric.target_ip: {
                "rtt": metric.rtt_ms,
                "loss": metric.loss_rate
            }
            for metric in data.metrics
        }
        
        # Store in topology database
        topology_db.store_telemetry(
            agent_id=data.agent_id,
            timestamp=data.timestamp,
            metrics=metrics_dict
        )
        
        # Log successful telemetry receipt
        logger.info(
            f"Received telemetry from agent {data.agent_id} "
            f"with {len(data.metrics)} metrics"
        )
        
        return JSONResponse(
            status_code=200,
            content={"status": "ok"}
        )
    
    except Exception as e:
        # Log error and return 400
        logger.error(f"Error processing telemetry: {str(e)}")
        raise HTTPException(
            status_code=400,
            detail=f"Invalid telemetry data: {str(e)}"
        )


@app.get("/api/v1/routes", response_model=RouteResponse, status_code=200)
async def get_routes(agent_id: str = Query(..., description="Agent ID requesting routes")) -> RouteResponse:
    """
    Retrieve route configurations for a specific agent.
    
    This endpoint computes optimal routes for the requesting agent based on
    current network topology and returns the route configurations.
    
    Args:
        agent_id: Unique identifier of the agent requesting routes
    
    Returns:
        RouteResponse containing list of route configurations
    
    Raises:
        HTTPException 404: If agent_id is not found in topology database
    
    Requirements:
        - 10.2: GET endpoint /api/v1/routes with agent_id query parameter
        - 10.4: Return HTTP 404 when agent_id is not found
        - 5.1: Return JSON response with next hop mappings
    
    Example Request:
        GET /api/v1/routes?agent_id=10.254.0.1
    
    Example Response:
        {
            "routes": [
                {
                    "dst_cidr": "10.254.0.3/32",
                    "next_hop": "10.254.0.2",
                    "reason": "optimized_path"
                },
                {
                    "dst_cidr": "10.254.0.4/32",
                    "next_hop": "direct",
                    "reason": "default"
                }
            ]
        }
    """
    try:
        # Check if agent exists in topology database
        if not topology_db.agent_exists(agent_id):
            logger.warning(f"Agent {agent_id} not found in topology database")
            raise HTTPException(
                status_code=404,
                detail=f"Agent not found. Has it sent telemetry?"
            )
        
        # Get all topology data for route computation
        topology_data = topology_db.get_all_data()
        
        # Compute routes for the requesting agent
        routes = route_solver.compute_routes_for(agent_id, topology_data)
        
        # Log route computation
        logger.info(
            f"Computed {len(routes)} routes for agent {agent_id}"
        )
        
        return RouteResponse(routes=routes)
    
    except HTTPException:
        # Re-raise HTTP exceptions (404)
        raise
    except Exception as e:
        # Log unexpected errors
        logger.error(f"Error computing routes for {agent_id}: {str(e)}")
        raise HTTPException(
            status_code=500,
            detail=f"Internal server error: {str(e)}"
        )


@app.get("/health")
async def health_check():
    """
    Health check endpoint.
    
    Returns basic system status information.
    """
    return {
        "status": "healthy",
        "agent_count": topology_db.get_agent_count()
    }


# For running the server directly
if __name__ == "__main__":
    import uvicorn
    uvicorn.run(app, host="0.0.0.0", port=8000)

"""
Pydantic data models for Lite SD-WAN Routing System.

These models define the data structures used for communication between
Agent and Controller, as well as internal data representation.
"""

from typing import List, Optional
from pydantic import BaseModel, Field, field_validator


class Metric(BaseModel):
    """
    Represents a single network quality metric for a target node.
    
    Attributes:
        target_ip: IP address of the target node being measured
        rtt_ms: Round-trip time in milliseconds (None if unreachable)
        loss_rate: Packet loss rate as a decimal (0.0 = no loss, 1.0 = 100% loss)
    """
    target_ip: str = Field(..., description="Target node IP address")
    rtt_ms: Optional[float] = Field(None, description="Round-trip time in milliseconds")
    loss_rate: float = Field(..., ge=0.0, le=1.0, description="Packet loss rate (0.0-1.0)")
    
    @field_validator('rtt_ms')
    @classmethod
    def validate_rtt(cls, v):
        """Ensure RTT is non-negative when present."""
        if v is not None and v < 0:
            raise ValueError("RTT must be non-negative")
        return v


class TelemetryRequest(BaseModel):
    """
    Telemetry data sent from Agent to Controller.
    
    Contains network quality measurements collected by the Agent's Prober module.
    
    Attributes:
        agent_id: Unique identifier for the reporting agent
        timestamp: Unix timestamp when measurements were collected
        metrics: List of network quality metrics for all peer nodes
    """
    agent_id: str = Field(..., description="Unique agent identifier")
    timestamp: int = Field(..., gt=0, description="Unix timestamp")
    metrics: List[Metric] = Field(..., min_length=1, description="Network quality metrics")


class RouteConfig(BaseModel):
    """
    Route configuration entry returned by Controller to Agent.
    
    Specifies how traffic to a destination should be routed.
    
    Attributes:
        dst_cidr: Destination IP address in CIDR notation (e.g., "10.254.0.3/32")
        next_hop: Next hop IP address, or "direct" for direct connections
        reason: Human-readable reason for this route (e.g., "optimized_path", "default")
    """
    dst_cidr: str = Field(..., description="Destination CIDR (e.g., 10.254.0.3/32)")
    next_hop: str = Field(..., description="Next hop IP or 'direct'")
    reason: str = Field(..., description="Reason for this route")
    
    @field_validator('dst_cidr')
    @classmethod
    def validate_cidr(cls, v):
        """Basic validation that CIDR contains a slash."""
        if '/' not in v:
            raise ValueError("dst_cidr must be in CIDR notation (e.g., 10.254.0.3/32)")
        return v


class RouteResponse(BaseModel):
    """
    Response from Controller containing route configurations for an Agent.
    
    Attributes:
        routes: List of route configurations to apply
    """
    routes: List[RouteConfig] = Field(default_factory=list, description="Route configurations")

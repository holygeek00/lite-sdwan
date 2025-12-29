# Controller REST API Documentation

## Overview

The Controller REST API provides endpoints for agents to send telemetry data and retrieve route configurations. The API is built with FastAPI and follows RESTful principles.

## Base URL

```
http://<controller-host>:8000
```

## Endpoints

### 1. POST /api/v1/telemetry

Receive network quality telemetry data from an agent.

**Requirements**: 10.1, 10.3, 3.5

**Request Body**:
```json
{
  "agent_id": "10.254.0.1",
  "timestamp": 1703830000,
  "metrics": [
    {
      "target_ip": "10.254.0.2",
      "rtt_ms": 35.5,
      "loss_rate": 0.0
    },
    {
      "target_ip": "10.254.0.3",
      "rtt_ms": 150.2,
      "loss_rate": 0.05
    }
  ]
}
```

**Response (200 OK)**:
```json
{
  "status": "ok"
}
```

**Error Responses**:
- `400 Bad Request`: Invalid JSON payload or validation error
- `422 Unprocessable Entity`: Pydantic validation error (e.g., negative RTT, loss_rate > 1.0)

**Example**:
```bash
curl -X POST http://localhost:8000/api/v1/telemetry \
  -H "Content-Type: application/json" \
  -d '{
    "agent_id": "10.254.0.1",
    "timestamp": 1703830000,
    "metrics": [
      {"target_ip": "10.254.0.2", "rtt_ms": 35.5, "loss_rate": 0.0}
    ]
  }'
```

---

### 2. GET /api/v1/routes

Retrieve route configurations for a specific agent.

**Requirements**: 10.2, 10.4, 5.1

**Query Parameters**:
- `agent_id` (required): Unique identifier of the agent requesting routes

**Response (200 OK)**:
```json
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
```

**Route Fields**:
- `dst_cidr`: Destination IP in CIDR notation (e.g., "10.254.0.3/32")
- `next_hop`: Next hop IP address, or "direct" for direct connections
- `reason`: Human-readable reason for this route
  - `"default"`: Direct connection (no relay needed)
  - `"optimized_path"`: Multi-hop path through relay node

**Error Responses**:
- `404 Not Found`: Agent ID not found in topology database
- `422 Unprocessable Entity`: Missing required query parameter

**Example**:
```bash
curl http://localhost:8000/api/v1/routes?agent_id=10.254.0.1
```

---

### 3. GET /health

Health check endpoint for monitoring.

**Response (200 OK)**:
```json
{
  "status": "healthy",
  "agent_count": 3
}
```

**Example**:
```bash
curl http://localhost:8000/health
```

---

## Data Models

### TelemetryRequest

```python
{
  "agent_id": str,        # Unique agent identifier
  "timestamp": int,       # Unix timestamp (must be > 0)
  "metrics": [            # List of metrics (min 1 entry)
    {
      "target_ip": str,   # Target node IP address
      "rtt_ms": float,    # Round-trip time in ms (>= 0 or null)
      "loss_rate": float  # Packet loss rate (0.0 - 1.0)
    }
  ]
}
```

### RouteResponse

```python
{
  "routes": [             # List of route configurations
    {
      "dst_cidr": str,    # Destination CIDR (e.g., "10.254.0.3/32")
      "next_hop": str,    # Next hop IP or "direct"
      "reason": str       # Reason for this route
    }
  ]
}
```

---

## Error Handling

### HTTP Status Codes

- `200 OK`: Request successful
- `400 Bad Request`: Invalid request data
- `404 Not Found`: Resource not found (e.g., agent_id)
- `422 Unprocessable Entity`: Validation error
- `500 Internal Server Error`: Unexpected server error

### Error Response Format

```json
{
  "detail": "Error message describing what went wrong"
}
```

---

## Running the Server

### Development Mode

```bash
python controller/api.py
```

### Production Mode

```bash
uvicorn controller.api:app --host 0.0.0.0 --port 8000
```

### With Auto-Reload (Development)

```bash
uvicorn controller.api:app --reload --host 0.0.0.0 --port 8000
```

---

## Testing

### Unit Tests

```bash
pytest tests/test_controller_api.py -v
```

### Manual Testing

```bash
python manual_test_controller.py
```

### Interactive API Documentation

FastAPI provides automatic interactive API documentation:

- Swagger UI: http://localhost:8000/docs
- ReDoc: http://localhost:8000/redoc

---

## Architecture

```
┌─────────────┐
│   Agent     │
└──────┬──────┘
       │ POST /api/v1/telemetry
       │ (send metrics)
       ↓
┌─────────────────────────────┐
│   Controller API            │
│  ┌────────────────────────┐ │
│  │  /api/v1/telemetry     │ │
│  │  - Validate data       │ │
│  │  - Store in DB         │ │
│  └────────────────────────┘ │
│  ┌────────────────────────┐ │
│  │  /api/v1/routes        │ │
│  │  - Check agent exists  │ │
│  │  - Compute routes      │ │
│  │  - Return config       │ │
│  └────────────────────────┘ │
└─────────────────────────────┘
       │
       ↓
┌─────────────────────────────┐
│   Topology Database         │
│   (In-Memory)               │
└─────────────────────────────┘
       │
       ↓
┌─────────────────────────────┐
│   Route Solver              │
│   (Dijkstra + Hysteresis)   │
└─────────────────────────────┘
```

---

## Configuration

The API uses the following default configuration:

- **Host**: 0.0.0.0 (all interfaces)
- **Port**: 8000
- **Penalty Factor**: 100 (for loss rate in cost calculation)
- **Hysteresis**: 0.15 (15% improvement threshold for route switching)

These can be modified in `controller/api.py` or through environment variables.

---

## Logging

The API logs the following events:

- Telemetry reception: `INFO` level
- Route computation: `INFO` level
- Agent not found: `WARNING` level
- Errors: `ERROR` level

Log format:
```
%(asctime)s - %(name)s - %(levelname)s - %(message)s
```

Example:
```
2025-12-29 17:34:38,795 - controller.api - INFO - Received telemetry from agent 10.254.0.1 with 2 metrics
```

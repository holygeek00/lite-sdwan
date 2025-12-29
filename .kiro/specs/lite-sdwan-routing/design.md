# Design Document: Lite SD-WAN Routing System

## Overview

本系统采用 **Controller-Agent 架构**，基于 WireGuard Overlay 网络实现分布式智能路由。系统分为控制面（Control Plane）和数据面（Data Plane）：

- **数据面**：WireGuard 提供加密的 Full Mesh 网络，所有节点通过虚拟内网 (10.254.0.0/24) 互联
- **控制面**：Controller 收集全网拓扑数据，运行图算法计算最优路径，Agent 执行路由决策

核心设计原则：
1. **集中控制，分布执行**：Controller 拥有全局视野做决策，Agent 本地执行路由变更
2. **最终一致性**：允许短暂的路由不一致，通过周期性同步达到一致状态
3. **故障降级**：Controller 不可用时，系统自动回退到 WireGuard 默认路由

## Architecture

### System Components

```
┌─────────────────────────────────────────────────────────────┐
│                    Control Plane                             │
│  ┌────────────────────────────────────────────────────┐     │
│  │              Controller (FastAPI)                   │     │
│  │  ┌──────────────┐  ┌──────────────┐  ┌──────────┐ │     │
│  │  │  Aggregator  │→ │ Topology DB  │→ │  Solver  │ │     │
│  │  │  (REST API)  │  │  (In-Memory) │  │(Dijkstra)│ │     │
│  │  └──────────────┘  └──────────────┘  └──────────┘ │     │
│  └────────────────────────────────────────────────────┘     │
└─────────────────────────────────────────────────────────────┘
                            ↑ ↓ HTTP
┌─────────────────────────────────────────────────────────────┐
│                      Data Plane                              │
│  ┌──────────────┐   ┌──────────────┐   ┌──────────────┐    │
│  │   Agent A    │   │   Agent B    │   │   Agent C    │    │
│  │ ┌──────────┐ │   │ ┌──────────┐ │   │ ┌──────────┐ │    │
│  │ │ Prober   │ │   │ │ Prober   │ │   │ │ Prober   │ │    │
│  │ └──────────┘ │   │ └──────────┘ │   │ └──────────┘ │    │
│  │ ┌──────────┐ │   │ ┌──────────┐ │   │ ┌──────────┐ │    │
│  │ │ Executor │ │   │ │ Executor │ │   │ │ Executor │ │    │
│  │ └──────────┘ │   │ └──────────┘ │   │ └──────────┘ │    │
│  └──────────────┘   └──────────────┘   └──────────────┘    │
│         ↕                  ↕                  ↕              │
│  ┌──────────────────────────────────────────────────────┐   │
│  │         WireGuard Full Mesh (10.254.0.0/24)          │   │
│  └──────────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────────┘
```

### Component Responsibilities

| Component | Location | Responsibilities |
|-----------|----------|------------------|
| **WireGuard Mesh** | All nodes | Encrypted UDP tunnels, default routing |
| **Agent - Prober** | All nodes | ICMP ping every 5s, MTR trace every 5min |
| **Agent - Executor** | All nodes | Fetch routes from Controller, modify kernel routing table |
| **Controller - Aggregator** | Central node | REST API endpoints, receive telemetry |
| **Controller - Topology DB** | Central node | Store network graph (nodes, edges, costs) |
| **Controller - Solver** | Central node | Dijkstra algorithm, compute next hops |

## Components and Interfaces

### 1. WireGuard Overlay Network

**Purpose**: Provide encrypted, full-mesh connectivity between all nodes.

**Configuration**:
- Network: `10.254.0.0/24`
- Interface: `wg0`
- Port: `51820/udp`
- Each node has a `[Peer]` section for every other node

**Key Parameters**:
```ini
[Interface]
PrivateKey = <node_private_key>
Address = 10.254.0.X/24
ListenPort = 51820

[Peer]
PublicKey = <peer_public_key>
Endpoint = <peer_public_ip>:51820
AllowedIPs = 10.254.0.Y/32
PersistentKeepalive = 25
```

**Kernel Requirements**:
```bash
sysctl -w net.ipv4.ip_forward=1
sysctl -w net.ipv4.conf.all.forwarding=1
```

### 2. Agent - Prober Module

**Purpose**: Collect real-time network quality metrics.

**Implementation**:
- Language: Python 3.9+
- Library: `ping3` for ICMP, `subprocess` for MTR
- Threading: Separate thread running in infinite loop

**Probe Logic**:
```python
class Prober:
    def __init__(self, peer_ips: List[str], interval: int = 5):
        self.peer_ips = peer_ips
        self.interval = interval
        self.metrics_buffer = deque(maxlen=10)  # Sliding window
    
    def probe_once(self, target_ip: str) -> Dict:
        # Send ICMP ping
        rtt = ping3.ping(target_ip, timeout=2)
        if rtt is None:
            return {"target": target_ip, "rtt_ms": None, "loss": 1.0}
        return {"target": target_ip, "rtt_ms": rtt * 1000, "loss": 0.0}
    
    def run(self):
        while True:
            for peer_ip in self.peer_ips:
                metric = self.probe_once(peer_ip)
                self.metrics_buffer.append(metric)
            time.sleep(self.interval)
```

**Metrics Smoothing**:
- Use moving average over last 10 measurements
- Formula: `avg_rtt = sum(rtt_samples) / len(rtt_samples)`
- Loss rate: `loss_rate = failed_pings / total_pings`

### 3. Agent - Executor Module

**Purpose**: Apply routing decisions to the Linux kernel.

**Implementation**:
```python
class Executor:
    def __init__(self, wg_interface: str = "wg0"):
        self.wg_interface = wg_interface
        self.current_routes = {}
    
    def get_current_routes(self) -> Dict[str, str]:
        # Parse output of: ip route show table main
        result = subprocess.run(
            ["ip", "route", "show", "table", "main"],
            capture_output=True, text=True
        )
        # Parse lines like: "10.254.0.3 via 10.254.0.2 dev wg0"
        routes = {}
        for line in result.stdout.splitlines():
            if "via" in line and self.wg_interface in line:
                parts = line.split()
                dst = parts[0]
                via = parts[2]
                routes[dst] = via
        return routes
    
    def apply_route(self, dst_ip: str, next_hop: str):
        if next_hop == "direct":
            # Remove specific route, fall back to WireGuard default
            subprocess.run(
                ["ip", "route", "del", f"{dst_ip}/32", "dev", self.wg_interface],
                check=False
            )
        else:
            # Add or replace relay route
            subprocess.run(
                ["ip", "route", "replace", f"{dst_ip}/32", 
                 "via", next_hop, "dev", self.wg_interface],
                check=True
            )
    
    def sync_routes(self, desired_routes: Dict[str, str]):
        current = self.get_current_routes()
        for dst, next_hop in desired_routes.items():
            if current.get(dst) != next_hop:
                self.apply_route(dst, next_hop)
```

**Error Handling**:
- If `ip route` command fails, log error and continue
- Retry on next sync cycle (10 seconds later)
- Never crash the agent process

### 4. Controller - Aggregator (REST API)

**Purpose**: Receive telemetry from agents and serve route configurations.

**Implementation**: FastAPI with in-memory storage

**Endpoints**:

#### POST /api/v1/telemetry
```python
from fastapi import FastAPI, HTTPException
from pydantic import BaseModel
from typing import List, Dict

app = FastAPI()

class Metric(BaseModel):
    target_ip: str
    rtt_ms: float | None
    loss_rate: float

class TelemetryRequest(BaseModel):
    agent_id: str
    timestamp: int
    metrics: List[Metric]

topology_db = {}  # Global in-memory store

@app.post("/api/v1/telemetry")
async def receive_telemetry(data: TelemetryRequest):
    # Store metrics in topology database
    topology_db[data.agent_id] = {
        "timestamp": data.timestamp,
        "metrics": {m.target_ip: {"rtt": m.rtt_ms, "loss": m.loss_rate} 
                    for m in data.metrics}
    }
    return {"status": "ok"}
```

#### GET /api/v1/routes?agent_id=<id>
```python
@app.get("/api/v1/routes")
async def get_routes(agent_id: str):
    if agent_id not in topology_db:
        raise HTTPException(status_code=404, detail="Agent not found")
    
    # Compute routes using Solver
    routes = solver.compute_routes_for(agent_id)
    return {"routes": routes}
```

### 5. Controller - Solver (Path Computation)

**Purpose**: Compute optimal paths using Dijkstra's algorithm.

**Implementation**:
```python
import networkx as nx

class RouteSolver:
    def __init__(self, penalty_factor: int = 100, hysteresis: float = 0.15):
        self.penalty_factor = penalty_factor
        self.hysteresis = hysteresis
        self.previous_costs = {}  # Track previous path costs
    
    def build_graph(self, topology_db: Dict) -> nx.DiGraph:
        G = nx.DiGraph()
        
        # Add all nodes
        for agent_id in topology_db.keys():
            G.add_node(agent_id)
        
        # Add edges with costs
        for source, data in topology_db.items():
            for target, metrics in data["metrics"].items():
                rtt = metrics["rtt"]
                loss = metrics["loss"]
                
                if rtt is None:  # Link is down
                    cost = float('inf')
                else:
                    cost = rtt + (loss * self.penalty_factor)
                
                G.add_edge(source, target, weight=cost)
        
        return G
    
    def compute_routes_for(self, source_agent: str) -> List[Dict]:
        G = self.build_graph(topology_db)
        routes = []
        
        for target in G.nodes():
            if target == source_agent:
                continue
            
            try:
                path = nx.shortest_path(G, source_agent, target, weight='weight')
                new_cost = nx.shortest_path_length(G, source_agent, target, weight='weight')
                
                # Apply hysteresis
                old_cost = self.previous_costs.get((source_agent, target), float('inf'))
                if new_cost < old_cost * (1 - self.hysteresis):
                    # Switch to new path
                    next_hop = path[1] if len(path) > 1 else "direct"
                    self.previous_costs[(source_agent, target)] = new_cost
                else:
                    # Keep old path (not enough improvement)
                    continue
                
                routes.append({
                    "dst_cidr": f"{target}/32",
                    "next_hop": next_hop,
                    "reason": "optimized_path" if next_hop != "direct" else "default"
                })
            except nx.NetworkXNoPath:
                # No path available, skip
                continue
        
        return routes

solver = RouteSolver()
```

**Cost Function**:
```
Cost = RTT_ms + (Loss_rate × Penalty_Factor)

Where:
- RTT_ms: Round-trip time in milliseconds
- Loss_rate: Packet loss as decimal (0.0 - 1.0)
- Penalty_Factor: 100 (makes 1% loss = 100ms penalty)
```

**Hysteresis Logic**:
- Only switch routes if new cost < old cost × (1 - 0.15)
- This means new path must be at least 15% better
- Prevents route flapping due to minor fluctuations

## Data Models

### Telemetry Data Structure
```python
{
    "agent_id": "node_shanghai",
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

### Route Configuration Structure
```python
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

### Topology Database Schema
```python
topology_db = {
    "10.254.0.1": {
        "timestamp": 1703830000,
        "metrics": {
            "10.254.0.2": {"rtt": 35.5, "loss": 0.0},
            "10.254.0.3": {"rtt": 150.2, "loss": 0.05}
        }
    },
    "10.254.0.2": {
        "timestamp": 1703830005,
        "metrics": {
            "10.254.0.1": {"rtt": 36.0, "loss": 0.0},
            "10.254.0.3": {"rtt": 80.0, "loss": 0.0}
        }
    }
}
```

## Correctness Properties

*A property is a characteristic or behavior that should hold true across all valid executions of a system—essentially, a formal statement about what the system should do. Properties serve as the bridge between human-readable specifications and machine-verifiable correctness guarantees.*


### Property 1: IP Address Subnet Validation
*For any* IP address used in the system configuration, the IP address should fall within the reserved subnet 10.254.0.0/24.
**Validates: Requirements 1.2**

### Property 2: Full Mesh Configuration Completeness
*For any* set of N nodes in the network, each node's configuration should contain exactly N-1 peer entries.
**Validates: Requirements 1.3**

### Property 3: RTT Calculation Correctness
*For any* ping response with valid timing data, the calculated RTT should be in milliseconds and non-negative.
**Validates: Requirements 2.2**

### Property 4: Packet Loss Rate Calculation
*For any* sequence of ping attempts with some failures, the calculated loss rate should be between 0.0 and 1.0 (inclusive) and equal to failed_pings / total_pings.
**Validates: Requirements 2.3**

### Property 5: Sliding Window Buffer Behavior
*For any* sequence of measurements added to a sliding window buffer with max size N, the buffer should never exceed N entries and should evict the oldest entry when full.
**Validates: Requirements 2.4**

### Property 6: Telemetry Serialization Round-Trip
*For any* valid probe data structure, serializing to JSON and then deserializing should produce an equivalent data structure.
**Validates: Requirements 3.1**

### Property 7: Telemetry Payload Completeness
*For any* telemetry payload sent to the Controller, it should contain agent_id, timestamp, and metrics fields with all target nodes included.
**Validates: Requirements 3.3**

### Property 8: Exponential Backoff Retry Logic
*For any* sequence of failed connection attempts, the retry delays should follow exponential backoff pattern (e.g., 1s, 2s, 4s) and stop after exactly 3 attempts.
**Validates: Requirements 3.4**

### Property 9: Topology Database Persistence
*For any* telemetry data received by the Controller, querying the topology database immediately after should return the same data.
**Validates: Requirements 3.5**

### Property 10: Link Cost Calculation Formula
*For any* latency value (in ms) and loss rate (0.0-1.0), the calculated cost should equal: latency + (loss_rate × 100).
**Validates: Requirements 4.1**

### Property 11: Graph Construction from Telemetry
*For any* set of telemetry data from N agents, the constructed graph should have N nodes and edges corresponding to all reported metrics.
**Validates: Requirements 4.2**

### Property 12: Next Hop Completeness
*For any* source node in the graph, every reachable destination node should have a computed next hop address (or "direct" for single-hop paths).
**Validates: Requirements 4.4**

### Property 13: Hysteresis Threshold Application
*For any* pair of old cost and new cost values, a route update should only be triggered if new_cost < old_cost × 0.85 (15% improvement threshold).
**Validates: Requirements 4.5, 7.1**

### Property 14: Route Response JSON Validity
*For any* route request to the Controller, the response should be valid JSON containing a "routes" array with properly formatted route objects.
**Validates: Requirements 5.1**

### Property 15: Direct Connection Indication
*For any* computed path with length 1 (source equals destination's neighbor), the next_hop field should be "direct".
**Validates: Requirements 5.2**

### Property 16: Relay Node IP Provision
*For any* computed path with length > 1, the next_hop field should contain the IP address of the second node in the path.
**Validates: Requirements 5.3**

### Property 17: Route Configuration Preservation on Error
*For any* current routing configuration, if the Controller returns an error response, the Agent's routing configuration should remain unchanged.
**Validates: Requirements 5.5**

### Property 18: Routing Table Diff Calculation
*For any* pair of desired routes and current routes, the diff calculation should correctly identify routes to add, modify, and delete.
**Validates: Requirements 6.1**

### Property 19: Route Command Generation
*For any* route change requirement (add relay or remove relay), the generated ip route command should have correct syntax: `ip route replace <target>/32 via <relay> dev wg0` or `ip route del <target>/32 dev wg0`.
**Validates: Requirements 6.2, 6.3**

### Property 20: Subnet Safety Constraint
*For any* route modification operation, the target IP address should be within the 10.254.0.0/24 subnet, otherwise the operation should be rejected.
**Validates: Requirements 6.5**

### Property 21: Moving Average Calculation
*For any* sequence of N measurements, the moving average should equal the sum of all measurements divided by N.
**Validates: Requirements 7.4**

### Property 22: Loop-Free Path Guarantee
*For any* path computed by Dijkstra's algorithm, the path should contain no repeated nodes (loop-free).
**Validates: Requirements 9.1**

### Property 23: Topology Graph Completeness
*For any* set of active agents, the Controller's topology graph should contain a node for each agent and edges for all reported peer connections.
**Validates: Requirements 9.4**

### Property 24: HTTP 200 Response with Valid JSON
*For any* valid API request to the Controller, the response should have HTTP status 200 and contain valid JSON that can be parsed without errors.
**Validates: Requirements 10.5**

### Property 25: Configuration Field Extraction
*For any* valid configuration file (YAML or JSON), parsing should correctly extract all required fields (Controller URL, intervals, listen address, etc.) without data loss.
**Validates: Requirements 11.2, 11.3**

## Error Handling

### Agent Error Handling

1. **Network Probe Failures**:
   - If ping times out (no response), record as 100% loss
   - Continue probing other peers
   - Never crash the probe thread

2. **Controller Communication Failures**:
   - Implement exponential backoff: 1s, 2s, 4s
   - After 3 failed attempts, enter fallback mode
   - In fallback mode: flush all dynamic routes, rely on WireGuard defaults
   - Periodically retry Controller connection (every 30s)

3. **Route Execution Failures**:
   - If `ip route` command fails, log error with full command and stderr
   - Do not crash the executor thread
   - Retry on next sync cycle (10s later)
   - If same route fails 5 times consecutively, skip it and alert

4. **Configuration Errors**:
   - Validate configuration on startup
   - If invalid, print clear error message and exit with code 1
   - Do not start with partial/invalid configuration

### Controller Error Handling

1. **Invalid Telemetry Data**:
   - Validate JSON schema using Pydantic
   - Return HTTP 400 with detailed error message
   - Do not store invalid data in topology database

2. **Missing Agent Data**:
   - If agent_id not found in topology database, return HTTP 404
   - Include helpful message: "Agent not found. Has it sent telemetry?"

3. **Graph Computation Failures**:
   - If no path exists between nodes, skip that route
   - If graph is empty, return empty routes array
   - Log warnings for disconnected components

4. **Algorithm Errors**:
   - Wrap Dijkstra computation in try-except
   - If NetworkX raises exception, log full traceback
   - Return last known good routes as fallback

## Testing Strategy

### Unit Testing Approach

We will use **pytest** as the testing framework for Python. Unit tests will focus on:

1. **Pure Functions**: Cost calculation, moving average, loss rate calculation
2. **Data Structures**: Sliding window buffer, topology database operations
3. **Command Generation**: Verify correct `ip route` commands are generated
4. **Configuration Parsing**: Test YAML/JSON parsing with various inputs
5. **Error Cases**: Invalid inputs, missing fields, malformed data

Example unit test structure:
```python
def test_cost_calculation():
    # Test specific examples
    assert calculate_cost(50.0, 0.0) == 50.0
    assert calculate_cost(50.0, 0.01) == 51.0
    assert calculate_cost(100.0, 0.1) == 110.0

def test_sliding_window_eviction():
    # Test edge case: buffer at max capacity
    buffer = SlidingWindow(maxlen=3)
    buffer.append(1)
    buffer.append(2)
    buffer.append(3)
    buffer.append(4)
    assert len(buffer) == 3
    assert list(buffer) == [2, 3, 4]
```

### Property-Based Testing Approach

We will use **Hypothesis** as the property-based testing library. Each correctness property will be implemented as a property test with minimum 100 iterations.

**Test Configuration**:
```python
from hypothesis import given, settings
import hypothesis.strategies as st

@settings(max_examples=100)
@given(latency=st.floats(min_value=0.0, max_value=1000.0),
       loss_rate=st.floats(min_value=0.0, max_value=1.0))
def test_property_cost_calculation(latency, loss_rate):
    """
    Feature: lite-sdwan-routing, Property 10: Link Cost Calculation Formula
    For any latency and loss rate, cost should equal latency + (loss_rate × 100)
    """
    cost = calculate_cost(latency, loss_rate)
    expected = latency + (loss_rate * 100)
    assert abs(cost - expected) < 0.001  # Float comparison tolerance
```

**Generator Strategies**:
- **IP Addresses**: Generate valid IPs in 10.254.0.0/24 range
- **Telemetry Data**: Generate valid telemetry payloads with random metrics
- **Graphs**: Generate random network topologies with 3-10 nodes
- **Routes**: Generate random route configurations
- **Measurements**: Generate sequences of RTT and loss measurements

**Key Property Tests**:
1. **Serialization Round-Trip** (Property 6): Generate random telemetry → serialize → deserialize → compare
2. **Hysteresis Logic** (Property 13): Generate random cost pairs → verify 15% threshold
3. **Loop-Free Paths** (Property 22): Generate random graphs → run Dijkstra → verify no repeated nodes
4. **Subnet Validation** (Property 1): Generate random IPs → verify subnet check
5. **Moving Average** (Property 21): Generate random measurement sequences → verify average calculation

### Integration Testing

Integration tests will verify component interactions:

1. **Agent ↔ Controller Communication**:
   - Start mock Controller
   - Agent sends telemetry
   - Verify Controller receives and stores data
   - Agent fetches routes
   - Verify correct routes returned

2. **End-to-End Route Update**:
   - Simulate 3-node network
   - Inject high latency on direct link
   - Verify Controller computes relay path
   - Verify Agent generates correct `ip route` command

3. **Fallback Mode**:
   - Start Agent with Controller down
   - Verify Agent enters fallback after 3 retries
   - Verify route flush command generated
   - Start Controller
   - Verify Agent recovers

### Test Coverage Goals

- **Unit Tests**: 80%+ code coverage
- **Property Tests**: All 25 correctness properties implemented
- **Integration Tests**: All critical paths covered
- **Edge Cases**: Empty graphs, single node, disconnected components

### Testing Tools

- **pytest**: Test runner and framework
- **Hypothesis**: Property-based testing library
- **pytest-cov**: Coverage reporting
- **pytest-mock**: Mocking for external dependencies (subprocess, network calls)
- **FastAPI TestClient**: API endpoint testing

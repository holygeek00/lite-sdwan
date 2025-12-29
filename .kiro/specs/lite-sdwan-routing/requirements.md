# Requirements Document

## Introduction

本系统是一个基于 Overlay 网络的分布式智能路由系统（Lite SD-WAN），用于在分布于不同地理位置的服务器之间构建全互联网络。系统通过实时监测节点间的链路质量（延迟、丢包），利用算法自动计算最优路径，并动态调整路由规则，实现网络加速与自动故障绕行。

## Glossary

- **Controller**: 中心控制器，负责汇聚全网拓扑数据、计算最优路径并提供 API 接口
- **Agent**: 部署在每个节点的客户端程序，负责探测链路质量和执行路由规则
- **Overlay_Network**: 基于 WireGuard 构建的虚拟内网，屏蔽底层公网差异
- **Prober**: Agent 中的探测模块，收集网络质量数据
- **Executor**: Agent 中的执行模块，修改 Linux 内核路由表
- **Full_Mesh**: 全互联网络拓扑，每个节点与其他所有节点直接连接
- **Cost_Function**: 链路成本计算公式，综合延迟和丢包率
- **Next_Hop**: 数据包到达目标节点的下一跳地址

## Requirements

### Requirement 1: Overlay 网络构建

**User Story:** 作为系统管理员，我希望在所有节点间建立安全的虚拟内网，以便节点可以通过加密隧道相互通信。

#### Acceptance Criteria

1. THE System SHALL use WireGuard to establish encrypted tunnels between all nodes
2. WHEN configuring the network, THE System SHALL assign IP addresses from the reserved subnet 10.254.0.0/24
3. THE System SHALL configure Full Mesh topology where each node connects to all other nodes
4. WHEN a node joins the network, THE System SHALL enable IP forwarding in the kernel
5. THE System SHALL configure firewall rules to allow WireGuard UDP traffic on port 51820

### Requirement 2: 链路质量探测

**User Story:** 作为系统运维人员，我希望实时了解节点间的网络质量，以便系统能够做出智能路由决策。

#### Acceptance Criteria

1. WHEN the Prober starts, THE Agent SHALL send ICMP ping packets to all peer nodes every 5 seconds
2. WHEN receiving ping responses, THE Prober SHALL calculate round-trip time (RTT) in milliseconds
3. WHEN packets are lost, THE Prober SHALL calculate packet loss rate as a percentage
4. THE Prober SHALL maintain a sliding window of recent measurements to smooth out network jitter
5. WHERE MTR integration is enabled, THE Agent SHALL execute MTR trace every 5 minutes to collect ASN information

### Requirement 3: 探测数据上报

**User Story:** 作为控制器，我需要接收所有节点的探测数据，以便构建全局网络拓扑图。

#### Acceptance Criteria

1. WHEN the Agent collects probe data, THE Agent SHALL format metrics as JSON payload
2. THE Agent SHALL send telemetry data to the Controller via HTTP POST every 10 seconds
3. WHEN sending telemetry, THE Agent SHALL include agent_id, timestamp, and metrics for all target nodes
4. IF the Controller is unreachable, THE Agent SHALL retry with exponential backoff up to 3 attempts
5. WHEN the Controller receives telemetry, THE Controller SHALL store the data in the topology database

### Requirement 4: 最优路径计算

**User Story:** 作为控制器，我需要计算任意两点间的最优路径，以便为节点提供路由决策。

#### Acceptance Criteria

1. WHEN calculating link cost, THE Controller SHALL use the formula: Cost = Latency_ms + (Loss_rate × 100)
2. THE Controller SHALL construct a weighted directed graph from all received telemetry data
3. WHEN computing optimal paths, THE Controller SHALL use Dijkstra's algorithm to find shortest paths
4. FOR ALL source-destination pairs, THE Controller SHALL determine the next hop address
5. WHEN a new path cost is lower than current path cost by at least 15%, THE Controller SHALL trigger a route update

### Requirement 5: 路由规则下发

**User Story:** 作为 Agent，我需要从控制器获取最新的路由配置，以便更新本地路由表。

#### Acceptance Criteria

1. WHEN the Agent requests routes, THE Controller SHALL return a JSON response with next hop mappings
2. THE Controller SHALL indicate direct connections with next_hop value "direct"
3. WHEN next_hop is a relay node, THE Controller SHALL provide the relay node's IP address
4. THE Agent SHALL poll the Controller for route updates every 10 seconds
5. IF the Controller returns an error, THE Agent SHALL continue using the current routing configuration

### Requirement 6: 路由表执行

**User Story:** 作为 Agent 执行模块，我需要将控制器的路由决策应用到内核路由表，以便实际流量按最优路径转发。

#### Acceptance Criteria

1. WHEN receiving route configuration, THE Executor SHALL compare it with the current kernel routing table
2. WHEN a relay route is required and not present, THE Executor SHALL execute `ip route replace <target>/32 via <relay> dev wg0`
3. WHEN a direct route is required and a relay route exists, THE Executor SHALL execute `ip route del <target>/32 dev wg0`
4. WHEN route changes fail, THE Executor SHALL log the error and retry on the next sync cycle
5. THE Executor SHALL only modify routes for the WireGuard subnet 10.254.0.0/24

### Requirement 7: 路由防抖机制

**User Story:** 作为系统设计者，我希望避免路由频繁切换，以便保持网络连接的稳定性。

#### Acceptance Criteria

1. WHEN evaluating a new path, THE Controller SHALL only switch routes if the new cost is at least 15% lower than the current cost
2. THE Controller SHALL maintain a hysteresis bias to prevent route flapping
3. WHEN network conditions stabilize, THE System SHALL wait for at least 3 consecutive measurement cycles before switching back to direct routes
4. THE System SHALL use moving average over the last N measurements to smooth out transient network fluctuations
5. WHEN a route switches, THE System SHALL record the timestamp to prevent re-switching within a minimum interval

### Requirement 8: 故障恢复与兜底

**User Story:** 作为系统管理员，我希望在控制器故障时系统仍能保持基本连通性，以便避免完全的网络中断。

#### Acceptance Criteria

1. IF the Agent fails to connect to the Controller for 3 consecutive attempts, THEN THE Agent SHALL enter fallback mode
2. WHEN entering fallback mode, THE Agent SHALL execute `ip route flush` to remove all dynamically added routes
3. WHEN routes are flushed, THE System SHALL rely on WireGuard's default Full Mesh routing
4. WHEN the Controller becomes reachable again, THE Agent SHALL resume normal operation and re-apply optimal routes
5. THE Agent SHALL log all fallback mode transitions for troubleshooting

### Requirement 9: 环路预防

**User Story:** 作为系统架构师，我需要确保路由决策不会产生环路，以便避免数据包无限循环。

#### Acceptance Criteria

1. WHEN computing paths with Dijkstra's algorithm, THE Controller SHALL guarantee loop-free routes
2. THE System SHALL rely on IP packet TTL mechanism as the final defense against routing loops
3. WHEN all Agents update routes, THE Controller SHALL ensure updates are synchronized within the same calculation cycle
4. THE Controller SHALL maintain a global view of the network topology to detect potential loops
5. IF a potential loop is detected in the graph, THE Controller SHALL log a warning and fall back to direct routing for affected nodes

### Requirement 10: API 接口实现

**User Story:** 作为开发者，我需要清晰定义的 RESTful API，以便 Agent 和 Controller 能够可靠通信。

#### Acceptance Criteria

1. THE Controller SHALL expose a POST endpoint `/api/v1/telemetry` for receiving probe data
2. THE Controller SHALL expose a GET endpoint `/api/v1/routes` with query parameter `agent_id` for route retrieval
3. WHEN receiving invalid JSON payloads, THE Controller SHALL return HTTP 400 with error details
4. WHEN an agent_id is not found, THE Controller SHALL return HTTP 404
5. THE Controller SHALL return HTTP 200 with valid JSON responses for successful requests

### Requirement 11: 配置与部署

**User Story:** 作为运维工程师，我希望系统易于部署和配置，以便快速在新节点上启动服务。

#### Acceptance Criteria

1. THE System SHALL provide a configuration file in YAML or JSON format for each component
2. THE Agent SHALL read configuration including Controller URL, probe interval, and sync interval
3. THE Controller SHALL read configuration including listen address, port, and algorithm parameters
4. THE System SHALL provide systemd service files for running Agent and Controller as daemons
5. THE System SHALL include a requirements.txt file listing all Python dependencies

### Requirement 12: 日志与监控

**User Story:** 作为系统管理员，我需要详细的日志记录，以便排查问题和监控系统运行状态。

#### Acceptance Criteria

1. THE Agent SHALL log all probe results including timestamp, target IP, RTT, and loss rate
2. THE Agent SHALL log all route changes including old route, new route, and reason
3. THE Controller SHALL log all received telemetry data with source agent_id
4. THE Controller SHALL log all route calculations including computed paths and costs
5. WHEN errors occur, THE System SHALL log error messages with sufficient context for debugging

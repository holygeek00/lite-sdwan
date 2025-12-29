这是一份为您整理的完整系统设计文档（Markdown 格式）。您可以直接复制保存为 `README.md` 或设计文档。

------

# 自建简易 SD-WAN 智能路由系统设计文档 (MVP)

## 1. 项目概述

本项目旨在构建一个基于 Overlay 网络的智能路由系统。针对 N 台分布在不同地理位置（不同 AS/ISP）的服务器，通过构建全互联网络，实时监测节点间的链路质量（延迟、丢包），利用算法自动计算最优路径，并动态调整路由规则，实现“网络加速”与“自动故障绕行”。

**核心目标：**

- **网络透明化：** 获得任意两点间的实时链路质量。
- **路径最优化：** 如果 A->B 的直连质量差，自动切换为 A->C->B。
- **配置自动化：** 全自动下发路由表，无需人工干预。

------

## 2. 系统架构 (Architecture)

系统采用 **“集中控制，分布执行”** 的架构，分为数据面（Data Plane）和控制面（Control Plane）。

### 2.1 架构图示

代码段

```
graph TD
    %% 样式定义
    classDef controller fill:#e3f2fd,stroke:#1565c0,stroke-width:2px;
    classDef agent fill:#e8f5e9,stroke:#2e7d32,stroke-width:2px;
    classDef infra fill:#fff3e0,stroke:#ef6c00,stroke-width:2px,stroke-dasharray: 5 5;

    subgraph Control_Plane ["控制面 (Control Plane)"]
        Central_Controller[("控制器 (Controller)")]:::controller
        Route_Solver["路径计算引擎 (Dijkstra)"]:::controller
        DB[(拓扑数据库)]:::controller
        Central_Controller <--> DB <--> Route_Solver
    end

    subgraph Data_Plane ["数据面 (Data Plane)"]
        Node_A["节点 A (Agent)"]:::agent
        Node_B["节点 B (Agent)"]:::agent
        Node_C["节点 C (Agent)"]:::agent
        
        WG_Net((WireGuard Full Mesh)):::infra
    end

    %% 交互关系
    Node_A --"1. 上报探测数据 (HTTP)"--> Central_Controller
    Node_B --"1. 上报探测数据 (HTTP)"--> Central_Controller
    Node_C --"1. 上报探测数据 (HTTP)"--> Central_Controller

    Central_Controller --"2. 下发最优路由 (JSON)"--> Node_A
    Central_Controller --"2. 下发最优路由 (JSON)"--> Node_B
    Central_Controller --"2. 下发最优路由 (JSON)"--> Node_C

    Node_A <--"3. 实际业务流量"--> WG_Net
    WG_Net <--"3. 实际业务流量"--> Node_B
```

### 2.2 核心组件

| **组件**              | **部署位置** | **职责**                                                     | **技术栈**                |
| --------------------- | ------------ | ------------------------------------------------------------ | ------------------------- |
| **Overlay 隧道**      | 所有节点     | 构建虚拟内网，屏蔽底层公网差异，提供加密传输。               | WireGuard                 |
| **Agent (探针)**      | 所有节点     | 1. **探测 (Prober):** 收集 Ping/Loss/MTR 数据。 2. **执行 (Executor):** 修改 Linux 内核路由表。 | Python, Ping3, IPRoute2   |
| **Controller (大脑)** | 中心节点     | 1. **汇聚:** 接收全网拓扑数据。 2. **计算:** 运行图算法计算最短路径。 3. **API:** 提供 RESTful 接口。 | Python, FastAPI, NetworkX |

------

## 3. 详细设计 (Detailed Design)

### 3.1 基础设施层 (Overlay Network)

使用 **WireGuard** 建立全互联（Full Mesh）网络。

- **IP 规划：** 使用保留网段（如 `10.10.0.0/24`）。
- **配置要点：**
  - 每台机器配置 `AllowedIPs` 包含所有其他 Peer 的内网 IP。
  - 开启内核转发：`net.ipv4.ip_forward=1`。
  - 防火墙放行 UDP 监听端口（默认 51820）。

### 3.2 探测机制 (Probe Logic)

Agent 端的探测器负责采集链路“成本”。

- **探测对象：** 针对 Mesh 网络中的所有 Peer IP 进行探测。
- **探测频率：** 高频探测（如 5秒/次）用于 Ping/丢包；低频探测（如 10分钟/次）用于 MTR/Trace 以获取 ASN 信息。
- **数据清洗：** 使用滑动窗口平均值（Moving Average）平滑抖动，避免路由频繁跳变。

### 3.3 路由决策算法 (Decision Engine)

Controller 使用加权有向图（Weighted Directed Graph）模型。

权重计算公式 (Cost Function):



$$Weight = Latency_{avg} + (LossRate \times PenaltyFactor)$$

- *Latency:* 毫秒级往返时延。
- *PenaltyFactor:* 丢包惩罚系数（例如 1000）。丢包 1% 代价应远大于延迟增加 10ms。

**算法步骤:**

1. 构建图 $G(V, E)$，其中 $V$ 是所有节点，$E$ 是探测到的链路，$Weight$ 是上述计算出的成本。
2. 针对每个请求节点 $S$，运行 **Dijkstra 算法** 计算到目标节点 $T$ 的最短路径。
3. 输出：目标 $T$ 的下一跳（Next Hop）地址。

### 3.4 路由执行 (Execution)

Agent 收到 Controller 的指令（如 `{"10.10.0.3": "10.10.0.2"}`，意为去往 .3 需经过 .2）后：

1. **检查当前路由表：** 读取 `ip route show table main`。

2. **比对差异：** * 如果指令为直连且当前无特殊规则 -> 不操作。

   - 如果指令为中转且当前为直连 -> 添加路由。
   - 如果指令改变了中转节点 -> 更新路由。

3. **命令示例：**

   Bash

   ```
   # 添加/修改中转路由
   ip route replace 10.10.0.3/32 via 10.10.0.2 dev wg0
   # 恢复直连（删除特定路由，回退到 WG 默认路由）
   ip route del 10.10.0.3/32 dev wg0
   ```

------

## 4. 数据结构与 API 设计

### 4.1 数据上报接口

- **Endpoint:** `POST /report`

- **Payload:**

  JSON

  ```
  {
    "source_ip": "10.10.0.1",
    "targets": {
      "10.10.0.2": { "rtt": 45.2, "loss": 0.0, "asn": "AS12345" },
      "10.10.0.3": { "rtt": 200.5, "loss": 5.0, "asn": "AS67890" }
    }
  }
  ```

### 4.2 路由拉取接口

- **Endpoint:** `GET /routes/{node_ip}`

- **Response:**

  JSON

  ```
  {
    "10.10.0.3": "10.10.0.2",  // 去往 .3，下一跳是 .2
    "10.10.0.4": "10.10.0.4"   // 去往 .4，直接走 .4 (直连)
  }
  ```

------

## 5. 稳定性与优化策略 (关键)

为了从 MVP 走向生产可用，必须处理以下边缘情况：

### 5.1 路由防抖 (Hysteresis)

网络波动是常态。为了防止路由在“直连”和“中转”之间疯狂切换：

- **策略：** 只有新路径的 Cost 比旧路径低 **X% (如 15%)** 或绝对延迟降低 **Y ms (如 20ms)** 时，才触发切换。

### 5.2 避免路由环路

- **场景：** A 认为去 C 走 B 好，B 认为去 C 走 A 好。导致 A->B->A->B 死循环。
- **解决：** * Controller 拥有全局视野，Dijkstra 算法本身保证计算出的静态路径无环。
  - **TTL 限制：** IP 包自带 TTL，防止无限循环。
  - **一致性周期：** 确保所有 Agent 的路由更新周期尽可能同步，或者由 Controller 统一推送版本号。

### 5.3 故障兜底

- 如果 Controller 挂了，Agent 无法拉取新路由怎么办？
- **策略：** 设置路由规则 TTL（本地缓存有效时间）。如果连接 Controller 失败，Agent 应自动运行 `ip route flush` 清除所有通过脚本添加的路由，**回退到 WireGuard 原始的全互联直连模式**，保证至少能通（哪怕质量差）。

------

## 6. 部署与后续规划

### 6.1 MVP 阶段 (当前)

- [x] 部署 WireGuard 组网。
- [x] 部署 Python Controller。
- [x] 部署 Python Agent (Ping + ip route)。

### 6.2 v1.0 增强阶段

- [ ] **集成 MTR:** 增加 ASN 维度，例如“流量不经过某个特定的 AS”。
- [ ] **流量控制:** 集成 `iptables` / `nftables`，不仅仅做路由，还做 NAT 转发（实现节点间流量共享，如作为出口网关）。
- [ ] **可视化:** 使用 Grafana + InfluxDB 展示实时拓扑和延迟热力图。

### 6.3 安全性

- Controller API 增加 Token 鉴权。
- 限制 Agent 仅能修改特定网段的路由，防止破坏宿主机网络。



# 项目名称：分布式智能路由系统 (Lite SD-WAN) 技术规格书

版本： v1.0 (MVP)

密级： 内部使用

## 1. 项目背景与目标

我们需要在 N 台分布在全球不同 IDC 的服务器之间，构建一个能够自动感知网络质量并动态调整路由的 Overlay 网络。

核心目标： 当两点间直连网络质量（延迟/丢包）恶化时，系统能自动计算出最优的中转节点，并下发路由规则，实现链路优化。

------

## 2. 总体架构设计

系统采用 **Controller-Agent** 架构，基于 **WireGuard** 构建数据平面。

### 2.1 技术栈选型

- **数据平面 (Data Plane):** WireGuard (UDP VPN), Linux Kernel IP Routing
- **控制平面 (Control Plane):** Python 3.9+ (FastAPI)
- **客户端 (Agent):** Python (Ping3, Subprocess), Linux Net-tools
- **数据存储:** 内存/Redis (MVP阶段), SQLite (持久化可选)

### 2.2 模块划分

| **模块**                    | **部署位置** | **职责描述**                                                 |
| --------------------------- | ------------ | ------------------------------------------------------------ |
| **Mesh Network**            | 所有节点     | 基于 WireGuard 组建的全互联内网（IP段 `10.254.0.0/24`）。    |
| **Agent - Prober**          | 所有节点     | 周期性探测其他节点的网络质量（Latency, Loss, Jitter）。      |
| **Agent - Executor**        | 所有节点     | 从 Controller 拉取路由表，执行 `ip route` 命令修改内核路由。 |
| **Controller - Aggregator** | 中心节点     | 接收全网探测数据，维护全局网络拓扑图。                       |
| **Controller - Solver**     | 中心节点     | 基于 Dijkstra 算法计算任意两点的最短路径（Cost）。           |

------

## 3. 详细实施方案

### 3.1 基础设施层 (WireGuard Overlay)

**要求：**

1. **网段规划：** 使用保留网段 `10.254.0.x/24`。

2. **Full Mesh 配置：** 每台机器的 WG 配置文件中必须包含**所有**其他 Peer 的 `[Peer]` 信息。

3. **内核参数：** 所有节点必须开启 IP 转发。

   Bash

   ```
   sysctl -w net.ipv4.ip_forward=1
   ```

4. **防火墙 (Iptables/NFT):** * 放行 UDP 监听端口（默认 51820）。

   - 配置转发规则（允许 wg0 接口进出的流量转发）。
   - `iptables -A FORWARD -i wg0 -j ACCEPT`

### 3.2 路由决策算法 (核心逻辑)

Controller 需维护一个**加权有向图**。

链路成本公式 (Cost Function):



$$Cost = Latency_{ms} + (Loss_{\%} \times Penalty\_Factor) + Hysteresis\_Bias$$

- **Latency:** 最近 N 次探测的平均 RTT。
- **Loss:** 最近 N 次探测的丢包率 (0.0 - 1.0)。
- **Penalty_Factor:** 惩罚系数，建议设为 **100**。即 1% 的丢包等同于增加 100ms 延迟。
- **Hysteresis (防抖):** 为了防止路由频繁切换，新路径的 Cost 必须比当前路径低 **15%** 以上才能生效。

### 3.3 Agent 端设计 (Python)

Agent 程序需作为 Systemd 服务常驻后台，包含两个并发线程：

#### A. 探测线程 (Probe Thread)

- **周期：** 每 5 秒一轮。
- **动作：** 对所有 Peer IP (除自己外) 发送 ICMP Ping (建议使用 `ping3` 库)。
- **MTR 集成 (可选):** 每 5 分钟执行一次 `mtr -j -z <IP>`，仅用于记录链路经过的 ASN，暂不参与实时路由计算。

#### B. 执行线程 (Sync Thread)

- **周期：** 每 10 秒一轮。

- **动作：**

  1. 打包探测数据 POST 给 Controller。

  2. 接收 Controller 返回的 `next_hop` 映射表。

  3. **Diff & Apply:**

     - 读取本机路由表 (`ip route show table main`)。

     - 若 API 返回去往 Target_IP 的下一跳是 Relay_IP，且当前路由表中不是这样，则执行：

       ip route replace <Target_IP>/32 via <Relay_IP> dev wg0

     - 若 API 返回直连，且当前有中转路由，则执行删除，恢复默认 WireGuard 路由。

------

## 4. 接口定义 (API Specs)

### 4.1 上报与心跳 (Report)

- **Method:** `POST /api/v1/telemetry`

- **Request Body:**

  JSON

  ```
  {
      "agent_id": "node_shanghai",
      "timestamp": 1703830000,
      "metrics": [
          { "target_ip": "10.254.0.2", "rtt_ms": 35.5, "loss_rate": 0.0 },
          { "target_ip": "10.254.0.3", "rtt_ms": 150.2, "loss_rate": 0.05 }
      ]
  }
  ```

### 4.2 拉取路由配置 (Pull Config)

- **Method:** `GET /api/v1/routes?agent_id=node_shanghai`

- **Response Body:**

  JSON

  ```
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

  *注：`next_hop` 为 "direct" 时，Agent 应删除针对该 IP 的特定路由规则，走系统默认。*

------

## 5. 开发阶段规划 (Roadmap)

请团队按以下三个阶段交付：

### 阶段一：基础组网与数据采集 (T+3天)

- [ ] 完成 3 台以上机器的 WireGuard Full Mesh 配置脚本。
- [ ] 完成 Agent 的探测模块，能输出 JSON 格式的 Latency/Loss 日志。
- [ ] 完成 Controller 的数据接收接口，能打印出全局拓扑数据。

### 阶段二：计算逻辑与路由下发 (T+7天)

- [ ] Controller 实现 Dijkstra 算法，输出路由表。
- [ ] Agent 实现 `ip route` 的修改逻辑（需处理权限问题，推荐使用 root 运行或 sudoers 配置）。
- [ ] 联调测试：人为制造直连丢包（使用 `tc` 命令模拟），观察路由是否自动切换。

### 阶段三：稳定性与防抖 (T+10天)

- [ ] 引入迟滞算法（防抖动）。
- [ ] 增加异常处理：Controller 宕机时，Agent 自动回退到直连模式。
- [ ] 简单的 Web UI 或控制台输出，展示当前的“最佳路径”状态。

------

## 6. 验收标准 (Definition of Done)

1. **功能验收：**
   - 在节点 A 使用 `tc qdisc add dev wg0 root netem loss 20%` 模拟丢包。
   - 系统应在 15秒内 自动将路由切换至无丢包的中转节点。
   - `mtr` 或 `traceroute` 显示路径发生变化。
   - 停止模拟丢包后，路由应在 1-2 分钟内切回直连。
2. **代码验收：**
   - Python 代码符合 PEP8 规范。
   - 包含 `requirements.txt` 和 `Dockerfile`（或一键部署脚本）。

------

## 7. 风险提示与对策

1. **路由环路：** 算法必须保证无环。TTL 机制是最后的防线。
2. **Controller 单点故障：** Agent 必须具备“失效模式”，连接不上 Controller 超过 3 次，强制清空所有动态路由规则，恢复默认。
3. **带宽消耗：** 探测包应尽量小，不要在该系统上传输大文件探测，仅做 ICMP Ping。
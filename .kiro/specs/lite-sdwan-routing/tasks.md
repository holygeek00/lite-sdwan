# Implementation Plan: Lite SD-WAN Routing System

## Overview

本实施计划将 SD-WAN 智能路由系统分解为可执行的开发任务。系统采用 Python 实现，包括 Controller（FastAPI）和 Agent（探测与执行）两个主要组件。任务按照依赖关系组织，从基础设施到核心功能，最后到集成测试。

## Tasks

- [x] 1. 项目初始化与基础设施
  - 创建项目目录结构（controller/, agent/, tests/, config/）
  - 创建 requirements.txt 包含所有依赖（FastAPI, uvicorn, ping3, networkx, hypothesis, pytest）
  - 创建配置文件模板（config.yaml）用于 Controller 和 Agent
  - 设置 pytest 配置文件（pytest.ini）
  - _Requirements: 11.1, 11.5_

- [x] 1.1 编写配置解析单元测试
  - 测试 YAML 配置文件解析
  - 测试缺失字段的错误处理
  - _Requirements: 11.2, 11.3_

- [x] 2. 实现核心数据模型
  - [x] 2.1 定义 Pydantic 数据模型
    - 创建 Metric 模型（target_ip, rtt_ms, loss_rate）
    - 创建 TelemetryRequest 模型（agent_id, timestamp, metrics）
    - 创建 RouteConfig 模型（dst_cidr, next_hop, reason）
    - _Requirements: 3.1, 5.1_

  - [x] 2.2 编写属性测试：Telemetry Serialization Round-Trip
    - **Property 6: Telemetry Serialization Round-Trip**
    - **Validates: Requirements 3.1**
    - 使用 Hypothesis 生成随机 telemetry 数据
    - 验证 JSON 序列化后反序列化得到等价对象
    - _Requirements: 3.1_

  - [x] 2.3 编写属性测试：Telemetry Payload Completeness
    - **Property 7: Telemetry Payload Completeness**
    - **Validates: Requirements 3.3**
    - 验证所有 telemetry payload 包含必需字段
    - _Requirements: 3.3_

- [x] 3. 实现 Agent - Prober 模块
  - [x] 3.1 实现 ICMP Ping 探测功能
    - 使用 ping3 库发送 ICMP 包
    - 计算 RTT（毫秒）
    - 计算丢包率
    - _Requirements: 2.1, 2.2, 2.3_

  - [x] 3.2 实现滑动窗口缓冲区
    - 使用 collections.deque 实现固定大小缓冲区
    - 实现移动平均计算
    - _Requirements: 2.4, 7.4_

  - [x] 3.3 编写单元测试：RTT 和丢包率计算
    - 测试正常响应的 RTT 计算
    - 测试超时情况的丢包率
    - _Requirements: 2.2, 2.3_

  - [x] 3.4 编写属性测试：Sliding Window Buffer Behavior
    - **Property 5: Sliding Window Buffer Behavior**
    - **Validates: Requirements 2.4**
    - 生成随机测量序列，验证缓冲区大小限制
    - _Requirements: 2.4_

  - [x] 3.5 编写属性测试：Moving Average Calculation
    - **Property 21: Moving Average Calculation**
    - **Validates: Requirements 7.4**
    - 生成随机测量序列，验证移动平均公式
    - _Requirements: 7.4_

  - [x] 3.6 编写属性测试：Packet Loss Rate Calculation
    - **Property 4: Packet Loss Rate Calculation**
    - **Validates: Requirements 2.3**
    - 生成随机 ping 结果序列，验证丢包率在 0.0-1.0 范围
    - _Requirements: 2.3_

- [x] 4. 实现 Agent - Executor 模块
  - [x] 4.1 实现路由表读取功能
    - 执行 `ip route show table main` 命令
    - 解析输出提取当前路由
    - _Requirements: 6.1_

  - [x] 4.2 实现路由命令生成
    - 生成 `ip route replace` 命令（中继路由）
    - 生成 `ip route del` 命令（恢复直连）
    - 添加子网安全检查（仅允许 10.254.0.0/24）
    - _Requirements: 6.2, 6.3, 6.5_

  - [x] 4.3 实现路由差异计算与应用
    - 比较期望路由与当前路由
    - 应用差异（添加/修改/删除路由）
    - 错误处理与重试逻辑
    - _Requirements: 6.1, 6.4_

  - [x] 4.4 编写属性测试：Route Command Generation
    - **Property 19: Route Command Generation**
    - **Validates: Requirements 6.2, 6.3**
    - 生成随机路由变更需求，验证命令语法正确
    - _Requirements: 6.2, 6.3_

  - [x] 4.5 编写属性测试：Subnet Safety Constraint
    - **Property 20: Subnet Safety Constraint**
    - **Validates: Requirements 6.5**
    - 生成随机 IP 地址，验证子网检查逻辑
    - _Requirements: 6.5_

  - [x] 4.6 编写属性测试：Routing Table Diff Calculation
    - **Property 18: Routing Table Diff Calculation**
    - **Validates: Requirements 6.1**
    - 生成随机路由配置对，验证 diff 算法
    - _Requirements: 6.1_

- [x] 5. 实现 Agent 主程序与通信逻辑
  - [x] 5.1 实现 HTTP 客户端
    - 使用 requests 库发送 POST /api/v1/telemetry
    - 使用 requests 库获取 GET /api/v1/routes
    - _Requirements: 3.2, 5.4_

  - [x] 5.2 实现指数退避重试机制
    - 实现 1s, 2s, 4s 的指数退避
    - 3 次失败后进入 fallback 模式
    - _Requirements: 3.4, 8.1_

  - [x] 5.3 实现 fallback 模式
    - 检测 Controller 不可达
    - 执行路由清空（ip route flush）
    - 定期重试连接 Controller
    - _Requirements: 8.1, 8.2, 8.4_

  - [x] 5.4 实现多线程协调
    - Prober 线程（5 秒周期）
    - Executor 线程（10 秒周期）
    - 线程安全的数据共享
    - _Requirements: 2.1, 3.2, 5.4_

  - [ ]* 5.5 编写属性测试：Exponential Backoff Retry Logic
    - **Property 8: Exponential Backoff Retry Logic**
    - **Validates: Requirements 3.4**
    - 模拟连接失败，验证重试延迟序列
    - _Requirements: 3.4_

  - [ ]* 5.6 编写单元测试：Fallback 模式转换
    - 测试 3 次失败后进入 fallback
    - 测试 Controller 恢复后退出 fallback
    - _Requirements: 8.1, 8.4_

  - [ ]* 5.7 编写属性测试：Route Configuration Preservation on Error
    - **Property 17: Route Configuration Preservation on Error**
    - **Validates: Requirements 5.5**
    - 模拟 Controller 错误响应，验证路由不变
    - _Requirements: 5.5_

- [x] 6. Checkpoint - Agent 功能验证
  - 确保所有 Agent 测试通过
  - 手动测试 Agent 能够探测本地网络
  - 如有问题请向用户反馈

- [x] 7. 实现 Controller - 拓扑数据库
  - [x] 7.1 实现内存拓扑数据库
    - 使用 Python 字典存储 agent_id -> metrics 映射
    - 实现数据存储与查询接口
    - _Requirements: 3.5_

  - [ ]* 7.2 编写属性测试：Topology Database Persistence
    - **Property 9: Topology Database Persistence**
    - **Validates: Requirements 3.5**
    - 生成随机 telemetry，存储后立即查询验证
    - _Requirements: 3.5_

- [x] 8. 实现 Controller - 路径计算引擎
  - [x] 8.1 实现链路成本计算
    - 实现公式：Cost = Latency_ms + (Loss_rate × 100)
    - _Requirements: 4.1_

  - [x] 8.2 实现图构建逻辑
    - 从拓扑数据库构建 NetworkX DiGraph
    - 添加节点和带权重的边
    - _Requirements: 4.2_

  - [x] 8.3 实现 Dijkstra 最短路径计算
    - 使用 NetworkX 的 shortest_path 函数
    - 为每个目标节点计算 next_hop
    - _Requirements: 4.3, 4.4_

  - [x] 8.4 实现迟滞（Hysteresis）逻辑
    - 维护历史成本记录
    - 仅当新成本 < 旧成本 × 0.85 时切换路由
    - _Requirements: 4.5, 7.1_

  - [ ]* 8.5 编写属性测试：Link Cost Calculation Formula
    - **Property 10: Link Cost Calculation Formula**
    - **Validates: Requirements 4.1**
    - 生成随机延迟和丢包率，验证成本公式
    - _Requirements: 4.1_

  - [ ]* 8.6 编写属性测试：Graph Construction from Telemetry
    - **Property 11: Graph Construction from Telemetry**
    - **Validates: Requirements 4.2**
    - 生成随机 telemetry 数据，验证图的节点和边
    - _Requirements: 4.2_

  - [ ]* 8.7 编写单元测试：Dijkstra 算法正确性
    - 使用已知图和已知最短路径测试
    - _Requirements: 4.3_

  - [ ]* 8.8 编写属性测试：Next Hop Completeness
    - **Property 12: Next Hop Completeness**
    - **Validates: Requirements 4.4**
    - 生成随机图，验证所有可达节点都有 next_hop
    - _Requirements: 4.4_

  - [ ]* 8.9 编写属性测试：Hysteresis Threshold Application
    - **Property 13: Hysteresis Threshold Application**
    - **Validates: Requirements 4.5, 7.1**
    - 生成随机成本对，验证 15% 阈值逻辑
    - _Requirements: 4.5, 7.1_

  - [ ]* 8.10 编写属性测试：Loop-Free Path Guarantee
    - **Property 22: Loop-Free Path Guarantee**
    - **Validates: Requirements 9.1**
    - 生成随机图，验证 Dijkstra 路径无重复节点
    - _Requirements: 9.1_

- [x] 9. 实现 Controller - REST API
  - [x] 9.1 实现 POST /api/v1/telemetry 端点
    - 接收 TelemetryRequest JSON
    - 验证数据格式（Pydantic）
    - 存储到拓扑数据库
    - 返回 200 OK 或 400 Bad Request
    - _Requirements: 10.1, 10.3, 3.5_

  - [x] 9.2 实现 GET /api/v1/routes 端点
    - 接收 agent_id 查询参数
    - 调用路径计算引擎
    - 返回路由配置 JSON
    - 处理 404 Not Found（agent_id 不存在）
    - _Requirements: 10.2, 10.4, 5.1_

  - [ ]* 9.3 编写单元测试：API 错误处理
    - 测试无效 JSON 返回 400
    - 测试不存在的 agent_id 返回 404
    - _Requirements: 10.3, 10.4_

  - [ ]* 9.4 编写属性测试：HTTP 200 Response with Valid JSON
    - **Property 24: HTTP 200 Response with Valid JSON**
    - **Validates: Requirements 10.5**
    - 生成随机有效请求，验证响应是 200 且 JSON 可解析
    - _Requirements: 10.5_

  - [ ]* 9.5 编写属性测试：Route Response JSON Validity
    - **Property 14: Route Response JSON Validity**
    - **Validates: Requirements 5.1**
    - 验证路由响应包含 "routes" 数组和正确格式
    - _Requirements: 5.1_

  - [ ]* 9.6 编写属性测试：Direct Connection Indication
    - **Property 15: Direct Connection Indication**
    - **Validates: Requirements 5.2**
    - 生成单跳路径，验证 next_hop 为 "direct"
    - _Requirements: 5.2_

  - [ ]* 9.7 编写属性测试：Relay Node IP Provision
    - **Property 16: Relay Node IP Provision**
    - **Validates: Requirements 5.3**
    - 生成多跳路径，验证 next_hop 是路径第二个节点
    - _Requirements: 5.3_

- [x] 10. Checkpoint - Controller 功能验证
  - 确保所有 Controller 测试通过
  - 使用 curl 或 Postman 手动测试 API 端点
  - 如有问题请向用户反馈

- [x] 11. 配置文件与部署脚本
  - [x] 11.1 创建 Agent 配置文件示例
    - 包含 controller_url, probe_interval, sync_interval, peer_ips
    - _Requirements: 11.2_

  - [x] 11.2 创建 Controller 配置文件示例
    - 包含 listen_address, port, penalty_factor, hysteresis
    - _Requirements: 11.3_

  - [x] 11.3 创建 systemd service 文件
    - agent.service 用于 Agent 守护进程
    - controller.service 用于 Controller 守护进程
    - _Requirements: 11.4_

  - [ ]* 11.4 编写属性测试：Configuration Field Extraction
    - **Property 25: Configuration Field Extraction**
    - **Validates: Requirements 11.2, 11.3**
    - 生成随机配置文件，验证所有字段正确提取
    - _Requirements: 11.2, 11.3_

  - [ ]* 11.5 编写单元测试：配置文件验证
    - 测试有效 YAML 配置解析
    - 测试缺失必需字段的错误
    - _Requirements: 11.1_

- [x] 12. 集成测试与端到端验证
  - [ ]* 12.1 编写集成测试：Agent-Controller 通信
    - 启动 mock Controller
    - Agent 发送 telemetry
    - 验证 Controller 接收并存储数据
    - Agent 获取路由
    - 验证返回正确路由配置
    - _Requirements: 3.2, 3.5, 5.1_

  - [ ]* 12.2 编写集成测试：端到端路由更新
    - 模拟 3 节点网络
    - 注入高延迟到直连链路
    - 验证 Controller 计算中继路径
    - 验证 Agent 生成正确 ip route 命令
    - _Requirements: 4.3, 6.2_

  - [ ]* 12.3 编写集成测试：Fallback 模式
    - 启动 Agent，Controller 关闭
    - 验证 Agent 3 次重试后进入 fallback
    - 验证生成路由清空命令
    - 启动 Controller
    - 验证 Agent 恢复正常
    - _Requirements: 8.1, 8.2, 8.4_

- [x] 13. 文档与最终验收
  - [x] 13.1 编写 README.md
    - 项目概述
    - 安装依赖（pip install -r requirements.txt）
    - 配置说明
    - 运行指南（启动 Controller 和 Agent）
    - 架构图

  - [x] 13.2 编写 WireGuard 配置指南
    - Full Mesh 配置示例
    - 内核参数设置
    - 防火墙规则

  - [x] 13.3 运行完整测试套件
    - 执行 pytest 确保所有测试通过
    - 生成覆盖率报告（pytest-cov）
    - 目标：80%+ 代码覆盖率

  - [x] 13.4 最终验收测试
    - 在 3 台虚拟机上部署 WireGuard
    - 启动 Controller 和 Agent
    - 使用 tc 命令模拟丢包
    - 验证路由自动切换（15 秒内）
    - 停止模拟，验证路由恢复（1-2 分钟内）

- [x] 14. Final Checkpoint - 项目完成
  - 确保所有测试通过
  - 确保文档完整
  - 向用户演示系统功能
  - 收集反馈并记录改进建议

## Notes

- 标记 `*` 的任务为可选测试任务，可以跳过以加快 MVP 开发
- 每个任务都引用了具体的需求编号，确保可追溯性
- Checkpoint 任务用于阶段性验证，确保增量进展
- 属性测试使用 Hypothesis 库，每个测试至少运行 100 次迭代
- 单元测试使用 pytest，关注具体示例和边界情况
- 集成测试验证组件间交互和端到端流程

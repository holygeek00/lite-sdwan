# Implementation Plan: Robustness Improvements

## Overview

本实现计划将 SD-WAN 系统的健壮性改进分解为可执行的编码任务，按照依赖关系排序，确保每个任务都能独立验证。

## Tasks

- [x] 1. 实现结构化日志模块
  - [x] 1.1 创建 `pkg/logging/logger.go` 定义 Logger 接口和 JSONLogger 实现
    - 定义 Level 类型和常量 (DEBUG, INFO, WARN, ERROR)
    - 定义 Field 结构体和 Logger 接口
    - 实现 JSONLogger 结构体
    - 实现 NewJSONLogger 构造函数
    - _Requirements: 4.1, 4.2, 4.3, 4.4_
  - [x] 1.2 实现日志级别过滤逻辑
    - 实现 shouldLog 方法判断是否输出
    - 实现 Debug/Info/Warn/Error 方法
    - 实现 WithFields 方法添加上下文
    - _Requirements: 4.5_
  - [x] 1.3 编写日志模块属性测试
    - **Property 5: JSON Log Format Consistency**
    - **Property 6: Log Level Filtering**
    - **Validates: Requirements 4.1, 4.2, 4.5**

- [x] 2. 实现配置验证模块
  - [x] 2.1 创建 `pkg/config/validator.go` 定义验证函数
    - 实现 ValidateIPAddress 函数
    - 实现 ValidateURL 函数
    - 实现 ValidatePort 函数
    - 实现 ValidateSubnet 函数
    - _Requirements: 1.1, 1.2, 1.3, 1.4_
  - [x] 2.2 实现 AgentConfig 和 ControllerConfig 验证
    - 实现 ValidateAgentConfig 函数
    - 实现 ValidateControllerConfig 函数
    - 返回 ValidationError 列表
    - _Requirements: 1.5, 1.6_
  - [x] 2.3 集成验证到启动流程
    - 修改 LoadAgentConfig 调用验证
    - 修改 LoadControllerConfig 调用验证
    - 验证失败时返回详细错误
    - _Requirements: 1.1, 1.2, 1.3, 1.4, 1.5, 1.6_
  - [ ]* 2.4 编写配置验证属性测试
    - **Property 1: IP Address Validation Correctness**
    - **Property 2: URL Validation Correctness**
    - **Property 3: Port Validation Correctness**
    - **Validates: Requirements 1.1, 1.2, 1.3, 1.4**

- [x] 3. Checkpoint - 验证日志和配置模块
  - 运行 `go test ./pkg/...` 确保所有测试通过
  - 确保代码通过 golangci-lint 检查

- [x] 4. 实现陈旧数据清理器
  - [x] 4.1 创建 `internal/controller/cleaner.go` 实现 StaleDataCleaner
    - 定义 StaleDataCleaner 结构体
    - 实现 NewStaleDataCleaner 构造函数
    - 实现 Start/Stop 方法控制清理循环
    - _Requirements: 3.1_
  - [x] 4.2 实现清理逻辑
    - 实现 cleanOnce 方法执行单次清理
    - 调用 TopologyDB.CleanStale 方法
    - 记录清理结果到日志
    - 维护清理计数指标
    - _Requirements: 3.2, 3.3, 3.4, 3.5, 3.6_
  - [x] 4.3 集成清理器到 Controller
    - 修改 Server 结构体添加 cleaner 字段
    - 在 NewServer 中创建并启动清理器
    - 在 Server 关闭时停止清理器
    - _Requirements: 3.1_
  - [ ]* 4.4 编写陈旧数据清理属性测试
    - **Property 4: Stale Data Detection and Cleanup**
    - **Validates: Requirements 3.2, 3.3, 3.5**

- [x] 5. 实现优雅关闭
  - [x] 5.1 修改 Agent 添加优雅关闭支持
    - 添加 Shutdown(ctx context.Context) 方法
    - 实现 cleanupRoutes 方法清理路由表
    - 实现 waitForInflight 方法等待请求完成
    - _Requirements: 2.1, 2.2, 2.3, 2.4_
  - [x] 5.2 修改 Agent.Run 使用优雅关闭
    - 创建带超时的 context
    - 调用 Shutdown 而不是 Stop
    - 记录关闭完成日志
    - _Requirements: 2.5, 2.6_
  - [x] 5.3 修改 Executor 添加路由追踪
    - 添加 managedRoutes 字段记录已添加的路由
    - 修改 SyncRoutes 更新 managedRoutes
    - 实现 CleanupManagedRoutes 方法
    - _Requirements: 2.3_
  - [ ]* 5.4 编写优雅关闭单元测试
    - 测试信号处理
    - 测试路由清理
    - 测试超时处理
    - **Validates: Requirements 2.1, 2.2, 2.3, 2.4, 2.5, 2.6**

- [x] 6. Checkpoint - 验证清理器和优雅关闭
  - 运行 `go test ./internal/...` 确保所有测试通过
  - 手动测试 Agent 关闭时路由清理

- [x] 7. 实现增强健康检查
  - [x] 7.1 创建健康检查数据结构
    - 在 `pkg/models/models.go` 添加 DetailedHealthResponse
    - 添加 ComponentHealth 结构体
    - _Requirements: 6.1, 6.2, 6.3, 6.4_
  - [x] 7.2 实现 Controller 详细健康检查
    - 修改 handleHealth 返回详细状态
    - 包含 TopologyDB 状态（节点数、最后更新时间）
    - 包含 Cleaner 状态（清理计数）
    - _Requirements: 6.1, 6.2_
  - [x] 7.3 实现 Agent 健康检查端点
    - 创建 `internal/agent/health.go`
    - 实现 /health 端点
    - 包含探测状态和 Controller 连接状态
    - _Requirements: 6.3, 6.4_
  - [x] 7.4 实现健康状态聚合逻辑
    - 任一组件不健康时返回 503
    - 所有组件健康时返回 200
    - _Requirements: 6.5_
  - [ ]* 7.5 编写健康检查属性测试
    - **Property 7: Health Check Response Time**
    - **Property 8: Health Status HTTP Code Mapping**
    - **Validates: Requirements 6.5, 6.6**

- [x] 8. 实现集成测试
  - [x] 8.1 创建 `tests/integration/agent_controller_test.go`
    - 设置测试 Controller 和 Agent
    - 测试遥测发送和路由接收
    - _Requirements: 5.1, 5.2_
  - [x] 8.2 实现 Fallback 模式集成测试
    - 测试 Controller 不可用时进入 fallback
    - 测试 Controller 恢复时退出 fallback
    - _Requirements: 5.3, 5.4_
  - [x] 8.3 实现路由更新集成测试
    - 使用 mock executor 验证路由应用
    - _Requirements: 5.5_
  - [ ]* 8.4 添加测试超时约束
    - 确保所有集成测试在 60 秒内完成
    - _Requirements: 5.6_

- [x] 9. 更新现有代码使用新模块
  - [x] 9.1 替换 Agent 中的 log 调用为 Logger
    - 修改 agent.go 使用结构化日志
    - 修改 prober.go 使用结构化日志
    - 修改 executor.go 使用结构化日志
    - 修改 client.go 使用结构化日志
    - _Requirements: 4.1, 4.6_
  - [x] 9.2 替换 Controller 中的 log 调用为 Logger
    - 修改 api.go 使用结构化日志
    - 修改 solver.go 使用结构化日志
    - 修改 topology_db.go 使用结构化日志
    - _Requirements: 4.2, 4.7_
  - [x] 9.3 更新 main.go 初始化日志
    - 从配置读取日志级别
    - 创建全局 Logger 实例
    - _Requirements: 4.3, 4.4_

- [x] 10. Final Checkpoint - 完整测试
  - 运行 `go test ./...` 确保所有测试通过
  - 运行 `golangci-lint run` 确保代码质量
  - 手动验证日志输出格式

## Notes

- 带 `*` 标记的任务为可选任务，可跳过以加快 MVP 开发
- 每个任务都引用了具体的需求条款以便追溯
- Checkpoint 任务用于阶段性验证
- 属性测试验证通用正确性属性
- 单元测试验证具体示例和边界情况

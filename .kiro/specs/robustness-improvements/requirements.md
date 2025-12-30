# Requirements Document

## Introduction

本文档定义了 SD-WAN 智能路由系统的健壮性改进需求，涵盖配置验证、优雅关闭、陈旧数据清理、结构化日志等关键功能，以提升系统在生产环境中的可靠性和可维护性。

## Glossary

- **Agent**: 部署在各节点上的代理程序，负责链路探测和路由执行
- **Controller**: 中央控制器，负责收集遥测数据并计算最优路由
- **Telemetry_Data**: Agent 上报的链路质量数据（RTT、丢包率）
- **Stale_Data**: 超过有效期的陈旧遥测数据
- **Graceful_Shutdown**: 优雅关闭，在程序退出前完成必要的清理工作
- **Structured_Logging**: 结构化日志，使用 JSON 格式输出便于解析和分析
- **Configuration_Validator**: 配置验证器，检查配置文件的格式和内容有效性

## Requirements

### Requirement 1: Configuration Validation

**User Story:** As a system administrator, I want the system to validate configuration files at startup, so that I can catch configuration errors early and avoid runtime failures.

#### Acceptance Criteria

1. WHEN the Agent starts with an invalid peer_ips format, THE Configuration_Validator SHALL return a descriptive error message and prevent startup
2. WHEN the Agent starts with an invalid controller_url format, THE Configuration_Validator SHALL return a descriptive error message and prevent startup
3. WHEN the Controller starts with an invalid listen_addr format, THE Configuration_Validator SHALL return a descriptive error message and prevent startup
4. WHEN the Agent starts with invalid port numbers (outside 1-65535), THE Configuration_Validator SHALL return a descriptive error message and prevent startup
5. WHEN the Agent starts with empty peer_ips list, THE Configuration_Validator SHALL return a descriptive error message and prevent startup
6. WHEN configuration validation fails, THE Configuration_Validator SHALL log the specific validation error with field name and expected format

### Requirement 2: Graceful Shutdown

**User Story:** As a system administrator, I want the Agent to clean up routes when shutting down, so that the routing table remains consistent after the Agent stops.

#### Acceptance Criteria

1. WHEN the Agent receives SIGTERM or SIGINT signal, THE Agent SHALL initiate graceful shutdown procedure
2. WHEN graceful shutdown is initiated, THE Agent SHALL stop accepting new probe results
3. WHEN graceful shutdown is initiated, THE Agent SHALL remove all routes that were added by the Agent from the routing table
4. WHEN graceful shutdown is initiated, THE Agent SHALL wait for in-flight HTTP requests to complete (with timeout)
5. WHEN graceful shutdown completes, THE Agent SHALL log a shutdown complete message
6. IF route cleanup fails during shutdown, THEN THE Agent SHALL log the error and continue with remaining cleanup tasks

### Requirement 3: Stale Data Cleanup

**User Story:** As a system administrator, I want the Controller to automatically clean up stale telemetry data, so that memory usage remains bounded and routing decisions are based on fresh data.

#### Acceptance Criteria

1. WHILE the Controller is running, THE Controller SHALL periodically check for stale telemetry data
2. WHEN telemetry data is older than the configured threshold (default 5 minutes), THE Controller SHALL mark it as stale
3. WHEN stale data is detected, THE Controller SHALL remove it from the topology database
4. WHEN stale data is removed, THE Controller SHALL log the cleanup action with affected node information
5. WHEN a node's all telemetry data becomes stale, THE Controller SHALL remove the node from the active topology
6. THE Controller SHALL expose a metric for the number of stale data cleanup operations

### Requirement 4: Structured Logging

**User Story:** As a system administrator, I want structured JSON logs with log levels, so that I can easily parse, filter, and analyze logs in production.

#### Acceptance Criteria

1. THE Agent SHALL output logs in JSON format with timestamp, level, message, and context fields
2. THE Controller SHALL output logs in JSON format with timestamp, level, message, and context fields
3. WHEN logging, THE Agent SHALL support log levels: DEBUG, INFO, WARN, ERROR
4. WHEN logging, THE Controller SHALL support log levels: DEBUG, INFO, WARN, ERROR
5. WHEN the log level is set to INFO, THE Agent SHALL suppress DEBUG level messages
6. WHEN logging errors, THE Agent SHALL include error context such as operation name, affected resource, and error details
7. WHEN logging HTTP requests, THE Controller SHALL include request method, path, status code, and duration

### Requirement 5: Integration Testing

**User Story:** As a developer, I want integration tests for Agent-Controller communication, so that I can verify the system works correctly end-to-end.

#### Acceptance Criteria

1. WHEN running integration tests, THE Test_Suite SHALL verify Agent can successfully send telemetry to Controller
2. WHEN running integration tests, THE Test_Suite SHALL verify Agent can successfully receive routes from Controller
3. WHEN running integration tests, THE Test_Suite SHALL verify Agent enters fallback mode when Controller is unavailable
4. WHEN running integration tests, THE Test_Suite SHALL verify Agent recovers from fallback mode when Controller becomes available
5. WHEN running integration tests, THE Test_Suite SHALL verify route updates are applied correctly to the routing table
6. WHEN running integration tests, THE Test_Suite SHALL complete within 60 seconds

### Requirement 6: Health Check Enhancement

**User Story:** As a system administrator, I want detailed health check endpoints, so that I can monitor the system's internal state and diagnose issues.

#### Acceptance Criteria

1. WHEN the /health endpoint is called, THE Controller SHALL return component-level health status
2. WHEN the /health endpoint is called, THE Controller SHALL include topology database status (node count, last update time)
3. WHEN the /health endpoint is called, THE Agent SHALL return probe status (last probe time, success rate)
4. WHEN the /health endpoint is called, THE Agent SHALL return controller connectivity status
5. WHEN any component is unhealthy, THE health endpoint SHALL return HTTP 503 with details
6. THE health endpoint SHALL respond within 100ms under normal conditions

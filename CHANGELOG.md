# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [1.1.0] - 2024-12-31

### ✨ 新功能

- **结构化日志**: JSON 格式日志输出，支持 DEBUG/INFO/WARN/ERROR 级别过滤
- **配置验证**: 启动时验证 IP 地址、URL、端口、子网等配置项
- **陈旧数据清理**: Controller 自动清理过期的遥测数据，保持内存使用稳定
- **优雅关闭**: Agent 关闭时自动清理已添加的路由，保持路由表一致性
- **增强健康检查**: 组件级别的健康状态，包含详细的诊断信息

### 🔧 改进

- 使用 `exec.CommandContext` 替代 `exec.Command`，支持命令超时
- 添加集成测试覆盖 Agent-Controller 通信场景
- 添加属性测试验证日志模块的正确性

### 📦 新增文件

- `pkg/logging/logger.go` - 结构化日志模块
- `pkg/config/validator.go` - 配置验证模块
- `internal/controller/cleaner.go` - 陈旧数据清理器
- `internal/agent/health.go` - Agent 健康检查端点
- `tests/integration/agent_controller_test.go` - 集成测试

---

## [1.0.0] - 2024-12-29

🎉 **首个正式版本发布！**

基于 WireGuard Overlay 网络的分布式智能路由系统，使用 Go 语言实现。

### ✨ 核心功能

- **智能路由**: 基于 Dijkstra 算法的最优路径计算
- **实时探测**: ICMP Ping 探测链路延迟和丢包率
- **自动切换**: 链路质量下降时自动切换到中继路由
- **路由防抖**: 15% 迟滞阈值防止路由频繁切换
- **故障恢复**: Controller 不可用时自动回退到 WireGuard 默认路由

### 🏗️ 架构

- **Controller**: REST API 服务，负责拓扑管理和路由计算
- **Agent**: 部署在每个节点，负责链路探测和路由执行
- **通信**: HTTP API + WireGuard Full Mesh

### 📦 部署方式

- **预编译二进制**: 支持 Linux/macOS/Windows，amd64/arm64/armv7
- **一键安装脚本**: `curl -sSL https://raw.githubusercontent.com/holygeek00/lite-sdwan/main/deploy/install.sh | sudo bash`
- **Docker 镜像**: `ghcr.io/holygeek00/lite-sdwan/controller` 和 `agent`
- **systemd 服务**: 开箱即用的服务配置

### 🔧 技术栈

- Go 1.21+
- Gin Web Framework
- go-ping (ICMP)
- WireGuard

### 📚 文档

- [README.md](README.md) - 快速开始
- [DEPLOYMENT_GUIDE.md](DEPLOYMENT_GUIDE.md) - 详细部署指南
- [WIREGUARD_GUIDE.md](WIREGUARD_GUIDE.md) - WireGuard 配置指南

---

[1.1.0]: https://github.com/holygeek00/lite-sdwan/releases/tag/v1.1.0
[1.0.0]: https://github.com/holygeek00/lite-sdwan/releases/tag/v1.0.0

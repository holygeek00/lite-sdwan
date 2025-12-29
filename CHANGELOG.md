# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added
- Initial Go implementation of Lite SD-WAN
- Controller with REST API (Gin framework)
- Agent with ICMP prober and route executor
- Dijkstra-based path computation
- Hysteresis mechanism for route stability
- Fallback mode when controller is unreachable
- systemd service files
- One-click deployment scripts
- GitHub Actions CI/CD pipeline
- Multi-platform binary builds (Linux, macOS, Windows)
- Docker images for controller and agent

### Changed
- Migrated from Python to Go for better performance and easier deployment

## [1.0.0] - 2024-XX-XX

### Added
- First stable release
- Full mesh WireGuard network support
- Real-time link quality monitoring
- Automatic route optimization
- Controller-Agent architecture
- REST API for telemetry and routes
- Configuration via YAML files
- Comprehensive documentation

[Unreleased]: https://github.com/example/lite-sdwan/compare/v1.0.0...HEAD
[1.0.0]: https://github.com/example/lite-sdwan/releases/tag/v1.0.0

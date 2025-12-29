# Lite SD-WAN Routing System

基于 WireGuard Overlay 网络的分布式智能路由系统（Lite SD-WAN）。使用 Go 语言实现，编译后为单一二进制文件，部署简单。

## 架构图

```
┌─────────────────────────────────────────────────────────────┐
│                    Control Plane                             │
│  ┌────────────────────────────────────────────────────┐     │
│  │              Controller (Gin)                       │     │
│  │  ┌──────────────┐  ┌──────────────┐  ┌──────────┐ │     │
│  │  │  REST API    │→ │ Topology DB  │→ │  Solver  │ │     │
│  │  │              │  │  (In-Memory) │  │(Dijkstra)│ │     │
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

## 核心特性

- **单一二进制**: Go 编译，无需安装运行时依赖
- **智能路由**: 基于 Dijkstra 算法计算最优路径
- **实时探测**: ICMP Ping 探测链路延迟和丢包率
- **自动切换**: 链路质量下降时自动切换到中继路由
- **故障恢复**: Controller 不可用时自动回退到 WireGuard 默认路由
- **路由防抖**: 15% 迟滞阈值防止路由频繁切换

## 项目结构

```
.
├── cmd/
│   ├── controller/        # Controller 主程序
│   └── agent/             # Agent 主程序
├── internal/
│   ├── controller/        # Controller 内部实现
│   │   ├── api.go         # REST API
│   │   ├── solver.go      # 路径计算引擎
│   │   └── topology_db.go # 拓扑数据库
│   └── agent/             # Agent 内部实现
│       ├── agent.go       # Agent 主逻辑
│       ├── prober.go      # 链路探测器
│       ├── executor.go    # 路由执行器
│       └── client.go      # HTTP 客户端
├── pkg/
│   ├── config/            # 配置解析
│   └── models/            # 数据模型
├── config/                # 配置文件示例
├── deploy/                # 部署脚本
├── systemd/               # systemd 服务文件
├── go.mod
├── go.sum
└── Makefile
```

## 快速开始

### 方式一：下载预编译二进制

```bash
# 下载最新版本
curl -LO https://github.com/example/lite-sdwan/releases/latest/download/sdwan-controller-linux-amd64
curl -LO https://github.com/example/lite-sdwan/releases/latest/download/sdwan-agent-linux-amd64

# 添加执行权限
chmod +x sdwan-*

# 移动到 PATH
sudo mv sdwan-controller-linux-amd64 /usr/local/bin/sdwan-controller
sudo mv sdwan-agent-linux-amd64 /usr/local/bin/sdwan-agent
```

### 方式二：从源码编译

```bash
# 克隆项目
git clone https://github.com/example/lite-sdwan.git
cd lite-sdwan

# 编译
make build

# 二进制文件在 build/ 目录
ls build/
# sdwan-controller  sdwan-agent
```

### 方式三：一键部署

```bash
# 运行部署向导
sudo ./deploy/deploy.sh
```

## 配置

### Controller 配置 (`/etc/sdwan/controller_config.yaml`)

```yaml
server:
  listen_address: "0.0.0.0"
  port: 8000

algorithm:
  penalty_factor: 100    # 丢包惩罚因子
  hysteresis: 0.15       # 切换阈值 (15%)

topology:
  stale_threshold: 60s   # 数据过期时间

logging:
  level: "INFO"
```

### Agent 配置 (`/etc/sdwan/agent_config.yaml`)

```yaml
agent_id: "10.254.0.1"

controller:
  url: "http://10.254.0.1:8000"
  timeout: 5s

probe:
  interval: 5s           # 探测周期
  timeout: 2s            # 探测超时
  window_size: 10        # 滑动窗口大小

sync:
  interval: 10s          # 同步周期
  retry_attempts: 3      # 重试次数
  retry_backoff: [1, 2, 4]  # 退避时间（秒）

network:
  wg_interface: "wg0"
  subnet: "10.254.0.0/24"
  peer_ips:
    - "10.254.0.2"
    - "10.254.0.3"
```

## 运行

### 启动 Controller

```bash
# 使用默认配置
sdwan-controller

# 指定配置文件
sdwan-controller -config /etc/sdwan/controller_config.yaml
```

### 启动 Agent（需要 root 权限）

```bash
# 使用默认配置
sudo sdwan-agent

# 指定配置文件
sudo sdwan-agent -config /etc/sdwan/agent_config.yaml
```

### 使用 systemd

```bash
# 安装服务
sudo cp systemd/sdwan-controller.service /etc/systemd/system/
sudo cp systemd/sdwan-agent.service /etc/systemd/system/
sudo systemctl daemon-reload

# 启动
sudo systemctl start sdwan-controller
sudo systemctl start sdwan-agent

# 开机自启
sudo systemctl enable sdwan-controller
sudo systemctl enable sdwan-agent
```

## API 文档

### POST /api/v1/telemetry

上报遥测数据。

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

### GET /api/v1/routes

获取路由配置。

```bash
curl "http://localhost:8000/api/v1/routes?agent_id=10.254.0.1"
```

### GET /health

健康检查。

```bash
curl http://localhost:8000/health
```

## 开发

### 运行测试

```bash
make test
```

### 生成覆盖率报告

```bash
make test-coverage
```

### 交叉编译

```bash
# Linux amd64
make build-linux

# Linux arm64
make build-linux-arm64

# 所有平台
make build-all
```

## 许可证

MIT License
# lite-sdwan

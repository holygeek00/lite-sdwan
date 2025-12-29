# Lite SD-WAN 一键部署系统

本目录包含 SD-WAN 系统的自动化部署工具。Go 语言实现，编译后为单一二进制文件，无需安装运行时依赖。

## 部署方式

### 方式一：一行命令安装（推荐）

```bash
# 自动下载预编译二进制，交互式配置
curl -sSL https://raw.githubusercontent.com/holygeek00/lite-sdwan/main/deploy/install.sh | sudo bash

# 或者使用 wget
wget -qO- https://raw.githubusercontent.com/holygeek00/lite-sdwan/main/deploy/install.sh | sudo bash
```

脚本会自动：
1. 检测系统架构 (amd64/arm64/armv7)
2. 下载预编译二进制文件
3. 安装 WireGuard
4. 引导你完成节点配置
5. 生成配置文件并启动服务

### 方式二：下载预编译二进制

```bash
# 下载最新版本
curl -LO https://github.com/holygeek00/lite-sdwan/releases/latest/download/sdwan-controller-linux-amd64
curl -LO https://github.com/holygeek00/lite-sdwan/releases/latest/download/sdwan-agent-linux-amd64

# 添加执行权限
chmod +x sdwan-*

# 移动到 PATH
sudo mv sdwan-controller-linux-amd64 /usr/local/bin/sdwan-controller
sudo mv sdwan-agent-linux-amd64 /usr/local/bin/sdwan-agent
```

### 方式三：从源码编译

```bash
# 克隆项目
git clone https://github.com/holygeek00/lite-sdwan.git
cd lite-sdwan

# 编译（需要 Go 1.21+）
make build

# 二进制文件在 build/ 目录
ls build/
# sdwan-controller  sdwan-agent
```

### 方式四：单节点交互式部署

```bash
# 克隆项目
git clone https://github.com/holygeek00/lite-sdwan.git
cd lite-sdwan

# 运行部署脚本
sudo ./deploy/deploy.sh
```

## 文件说明

| 文件 | 说明 |
|------|------|
| `install.sh` | 一键安装脚本（推荐，自动下载二进制） |
| `deploy.sh` | 单节点交互式部署脚本（从源码编译） |
| `batch_deploy.sh` | 批量远程部署脚本 |
| `quick_install.sh` | 克隆仓库后运行部署 |
| `nodes.yaml.example` | 批量部署配置模板 |

## 部署流程

```
┌─────────────────────────────────────────────────────────────┐
│                    install.sh 执行流程                       │
├─────────────────────────────────────────────────────────────┤
│  1. 检测操作系统和架构                                       │
│  2. 安装依赖 (curl, wget, wireguard-tools)                  │
│  3. 下载预编译二进制文件                                     │
│     - 如果下载失败，自动从源码编译                           │
│  4. 生成 WireGuard 密钥对                                   │
│  5. 交互式配置向导                                          │
│     - 选择角色 (Controller/Agent)                           │
│     - 配置 WireGuard IP 和公网 IP                           │
│     - 添加对等节点信息                                       │
│  6. 配置内核参数 (IP 转发)                                  │
│  7. 生成配置文件                                            │
│     - /etc/wireguard/wg0.conf                              │
│     - /etc/sdwan/agent_config.yaml                         │
│     - /etc/sdwan/controller_config.yaml (Controller)       │
│  8. 安装 systemd 服务                                       │
│  9. 启动服务                                                │
└─────────────────────────────────────────────────────────────┘
```

## 配置文件格式

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
  retry_backoff: [1, 2, 4]

network:
  wg_interface: "wg0"
  subnet: "10.254.0.0/24"
  peer_ips:
    - "10.254.0.2"
    - "10.254.0.3"
```

## 前置要求

### 目标节点
- Linux 系统（Ubuntu 18.04+, CentOS 7+, Debian 10+, Fedora, Arch）
- Root 权限
- 公网 IP 或可互相访问的网络

### 支持的架构
- linux/amd64 (x86_64)
- linux/arm64 (aarch64)
- linux/armv7 (树莓派等)

## 部署后验证

### 检查 WireGuard 状态
```bash
sudo wg show
```

### 检查服务状态
```bash
# Agent 状态
sudo systemctl status sdwan-agent

# Controller 状态（仅中心节点）
sudo systemctl status sdwan-controller
```

### 测试 API
```bash
# 健康检查
curl http://localhost:8000/health

# 获取路由
curl "http://localhost:8000/api/v1/routes?agent_id=10.254.0.2"
```

### 查看日志
```bash
# Agent 日志
sudo journalctl -u sdwan-agent -f

# Controller 日志
sudo journalctl -u sdwan-controller -f
```

## 故障排查

### WireGuard 无法启动
```bash
# 检查配置文件
cat /etc/wireguard/wg0.conf

# 手动启动调试
wg-quick up wg0
```

### 服务启动失败
```bash
# 查看详细错误
sudo journalctl -u sdwan-agent -n 50

# 检查配置文件
cat /etc/sdwan/agent_config.yaml

# 手动运行测试
sudo /usr/local/bin/sdwan-agent -config /etc/sdwan/agent_config.yaml
```

### 连接 Controller 失败
```bash
# 检查 Controller 是否运行
curl http://<controller-ip>:8000/health

# 检查防火墙
sudo iptables -L -n | grep 8000
```

## 卸载

```bash
# 停止服务
sudo systemctl stop sdwan-agent sdwan-controller
sudo systemctl disable sdwan-agent sdwan-controller

# 停止 WireGuard
sudo wg-quick down wg0
sudo systemctl disable wg-quick@wg0

# 删除文件
sudo rm -f /usr/local/bin/sdwan-controller /usr/local/bin/sdwan-agent
sudo rm -rf /etc/sdwan
sudo rm -f /etc/systemd/system/sdwan-*.service
sudo systemctl daemon-reload
```

## 安全建议

1. **防火墙**: 仅开放必要端口
   - 51820/UDP - WireGuard
   - 8000/TCP - Controller API（建议仅内网访问）
2. **网络隔离**: Controller API 应仅在 WireGuard 内网访问
3. **定期更新**: 关注 GitHub Releases 获取安全更新

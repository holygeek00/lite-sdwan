# 最终验收测试指南

本文档描述如何在 3 台虚拟机上进行 SD-WAN 系统的最终验收测试。

## 测试环境要求

### 硬件/虚拟机

- 3 台 Linux 虚拟机（Ubuntu 20.04+ 或 CentOS 8+）
- 每台 VM 至少 1GB RAM, 1 vCPU
- 所有 VM 之间网络互通

### 网络规划

| 节点 | 公网 IP | WireGuard IP | 角色 |
|------|---------|--------------|------|
| VM1 | 192.168.1.101 | 10.254.0.1 | Controller + Agent |
| VM2 | 192.168.1.102 | 10.254.0.2 | Agent |
| VM3 | 192.168.1.103 | 10.254.0.3 | Agent |

## 测试步骤

### 1. 部署 WireGuard

在所有 3 台 VM 上按照 `WIREGUARD_GUIDE.md` 配置 WireGuard Full Mesh。

验证连通性：
```bash
# 在每台 VM 上测试
ping -c 3 10.254.0.1
ping -c 3 10.254.0.2
ping -c 3 10.254.0.3
```

### 2. 部署 Controller（VM1）

```bash
# 安装依赖
pip install -r requirements.txt

# 配置 Controller
cat > config/controller_config.yaml << EOF
server:
  listen_address: "0.0.0.0"
  port: 8000
algorithm:
  penalty_factor: 100
  hysteresis: 0.15
EOF

# 启动 Controller
python -m uvicorn controller.api:app --host 0.0.0.0 --port 8000
```

### 3. 部署 Agent（所有 VM）

在 VM1 上：
```bash
cat > config/agent_config.yaml << EOF
agent_id: "10.254.0.1"
controller:
  url: "http://10.254.0.1:8000"
probe:
  interval: 5
sync:
  interval: 10
network:
  interface: "wg0"
  peer_ips:
    - "10.254.0.2"
    - "10.254.0.3"
EOF

sudo python -m agent.main config/agent_config.yaml
```

在 VM2 上：
```bash
cat > config/agent_config.yaml << EOF
agent_id: "10.254.0.2"
controller:
  url: "http://10.254.0.1:8000"
probe:
  interval: 5
sync:
  interval: 10
network:
  interface: "wg0"
  peer_ips:
    - "10.254.0.1"
    - "10.254.0.3"
EOF

sudo python -m agent.main config/agent_config.yaml
```

在 VM3 上：
```bash
cat > config/agent_config.yaml << EOF
agent_id: "10.254.0.3"
controller:
  url: "http://10.254.0.1:8000"
probe:
  interval: 5
sync:
  interval: 10
network:
  interface: "wg0"
  peer_ips:
    - "10.254.0.1"
    - "10.254.0.2"
EOF

sudo python -m agent.main config/agent_config.yaml
```

### 4. 验证系统正常运行

检查 Controller 接收到 telemetry：
```bash
curl http://10.254.0.1:8000/api/v1/routes?agent_id=10.254.0.1
```

检查路由表：
```bash
ip route show dev wg0
```

### 5. 模拟丢包测试

在 VM2 上模拟到 VM3 的高丢包：
```bash
# 添加 50% 丢包
sudo tc qdisc add dev wg0 root netem loss 50%

# 或者针对特定目标
sudo iptables -A OUTPUT -d 10.254.0.3 -m statistic --mode random --probability 0.5 -j DROP
```

### 6. 验证路由自动切换

等待 15 秒后，检查 VM2 的路由表：
```bash
ip route show dev wg0
```

预期结果：应该看到到 10.254.0.3 的路由通过 10.254.0.1 中继：
```
10.254.0.3 via 10.254.0.1 dev wg0
```

### 7. 停止模拟，验证路由恢复

```bash
# 移除丢包模拟
sudo tc qdisc del dev wg0 root

# 或移除 iptables 规则
sudo iptables -D OUTPUT -d 10.254.0.3 -m statistic --mode random --probability 0.5 -j DROP
```

等待 1-2 分钟（迟滞机制），检查路由是否恢复直连：
```bash
ip route show dev wg0
```

预期结果：到 10.254.0.3 的中继路由应该被移除，恢复直连。

### 8. 测试 Controller 故障恢复

在 VM1 上停止 Controller：
```bash
# Ctrl+C 停止 Controller
```

等待 30 秒，检查 Agent 日志，应该看到进入 fallback 模式。

检查路由表：
```bash
ip route show dev wg0
```

预期结果：所有动态添加的路由应该被清除。

重新启动 Controller：
```bash
python -m uvicorn controller.api:app --host 0.0.0.0 --port 8000
```

等待 30 秒，Agent 应该恢复正常并重新应用最优路由。

## 验收标准

| 测试项 | 预期结果 | 通过/失败 |
|--------|----------|-----------|
| WireGuard Full Mesh 连通 | 所有节点可互相 ping | |
| Controller 接收 telemetry | API 返回 200 | |
| Agent 获取路由 | API 返回路由配置 | |
| 丢包时路由切换 | 15 秒内切换到中继路由 | |
| 恢复后路由回退 | 1-2 分钟内恢复直连 | |
| Controller 故障 fallback | Agent 清除动态路由 | |
| Controller 恢复 | Agent 重新应用路由 | |

## 故障排查

### Agent 无法连接 Controller

```bash
# 检查 Controller 是否运行
curl http://10.254.0.1:8000/health

# 检查防火墙
sudo iptables -L -n | grep 8000
```

### 路由未生效

```bash
# 检查 Agent 日志
journalctl -u sdwan-agent -f

# 检查 Agent 是否有 root 权限
sudo python -m agent.main config/agent_config.yaml
```

### 探测失败

```bash
# 检查 WireGuard 状态
sudo wg show

# 手动测试 ping
ping -c 3 10.254.0.2
```

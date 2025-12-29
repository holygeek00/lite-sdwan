# Lite SD-WAN 一键部署系统

本目录包含 SD-WAN 系统的自动化部署工具。

## 部署方式

### 方式一：单节点交互式部署

在每个节点上运行部署脚本，按照向导完成配置：

```bash
# 下载项目
git clone https://github.com/your-repo/lite-sdwan.git
cd lite-sdwan

# 运行部署脚本
sudo ./deploy/deploy.sh
```

部署向导会引导你完成：
1. 选择节点角色（Controller 或 Agent）
2. 配置 WireGuard IP 和公网 IP
3. 添加对等节点信息
4. 自动安装依赖和配置服务

### 方式二：批量远程部署

从一台管理机器通过 SSH 批量部署到所有节点：

```bash
# 1. 复制配置模板
cp deploy/nodes.yaml.example deploy/nodes.yaml

# 2. 编辑配置文件，填入所有节点信息
vim deploy/nodes.yaml

# 3. 运行批量部署
./deploy/batch_deploy.sh deploy/nodes.yaml
```

### 方式三：一行命令安装

```bash
# 推荐方式 - 自动下载预编译二进制
curl -sSL https://raw.githubusercontent.com/holygeek00/lite-sdwan/main/deploy/install.sh | sudo bash

# 或者使用 wget
wget -qO- https://raw.githubusercontent.com/holygeek00/lite-sdwan/main/deploy/install.sh | sudo bash
```

## 文件说明

| 文件 | 说明 |
|------|------|
| `install.sh` | 一键安装脚本（推荐） |
| `deploy.sh` | 单节点交互式部署脚本 |
| `batch_deploy.sh` | 批量远程部署脚本 |
| `quick_install.sh` | 克隆仓库后运行部署 |
| `nodes.yaml.example` | 批量部署配置模板 |

## 部署流程

### 单节点部署流程

```
┌─────────────────────────────────────────────────────────────┐
│                    deploy.sh 执行流程                        │
├─────────────────────────────────────────────────────────────┤
│  1. 检测操作系统                                             │
│  2. 安装系统依赖 (Python, WireGuard)                        │
│  3. 生成 WireGuard 密钥对                                   │
│  4. 交互式获取节点配置                                       │
│  5. 配置内核参数 (IP 转发)                                  │
│  6. 生成配置文件                                            │
│     - /etc/wireguard/wg0.conf                              │
│     - /etc/sdwan/agent_config.yaml                         │
│     - /etc/sdwan/controller_config.yaml (Controller)       │
│  7. 配置防火墙                                              │
│  8. 安装 systemd 服务                                       │
│  9. 启动服务                                                │
│ 10. 验证部署                                                │
└─────────────────────────────────────────────────────────────┘
```

### 批量部署流程

```
┌─────────────────────────────────────────────────────────────┐
│                 batch_deploy.sh 执行流程                     │
├─────────────────────────────────────────────────────────────┤
│  1. 解析 nodes.yaml 配置文件                                │
│  2. 为所有节点生成 WireGuard 密钥                           │
│  3. 生成所有节点的配置文件                                   │
│  4. 通过 SSH 部署到每个节点                                 │
│     - 复制项目文件                                          │
│     - 安装依赖                                              │
│     - 配置内核参数                                          │
│  5. 安装 systemd 服务                                       │
│  6. 按顺序启动服务                                          │
│     - 先启动 Controller                                     │
│     - 再启动所有 Agent                                      │
└─────────────────────────────────────────────────────────────┘
```

## 配置文件格式

### nodes.yaml

```yaml
# Controller 节点
controller:
  host: 1.2.3.4              # 公网 IP
  ssh_user: root             # SSH 用户名
  ssh_port: 22               # SSH 端口
  wg_ip: 10.254.0.1          # WireGuard 内网 IP

# Agent 节点列表
agents:
  - host: 5.6.7.8
    ssh_user: root
    ssh_port: 22
    wg_ip: 10.254.0.2

  - host: 9.10.11.12
    ssh_user: root
    ssh_port: 22
    wg_ip: 10.254.0.3
```

## 前置要求

### 管理机器（批量部署）
- SSH 客户端
- Python 3.6+
- WireGuard 工具 (`wg` 命令)

### 目标节点
- Linux 系统（Ubuntu 18.04+, CentOS 7+, Debian 10+）
- Root SSH 访问权限
- 公网 IP 或可互相访问的网络

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
curl http://<controller-ip>:8000/health

# 获取路由
curl http://<controller-ip>:8000/api/v1/routes?agent_id=10.254.0.2
```

### 查看日志
```bash
# Agent 日志
sudo journalctl -u sdwan-agent -f

# Controller 日志
sudo journalctl -u sdwan-controller -f
```

## 故障排查

### SSH 连接失败
```bash
# 检查 SSH 连接
ssh -v root@<host>

# 确保 SSH 密钥已配置
ssh-copy-id root@<host>
```

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
sudo rm -rf /opt/sdwan /etc/sdwan
sudo rm /etc/systemd/system/sdwan-*.service
sudo systemctl daemon-reload
```

## 安全建议

1. **SSH 密钥认证**: 使用 SSH 密钥而非密码
2. **防火墙**: 仅开放必要端口 (51820/UDP, 8000/TCP)
3. **网络隔离**: Controller API 应仅在内网访问
4. **定期更新**: 定期更新系统和依赖包

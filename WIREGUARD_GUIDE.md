# WireGuard 配置指南

本指南介绍如何为 Lite SD-WAN 系统配置 WireGuard Full Mesh 网络。

## 概述

系统使用 WireGuard 作为 Overlay 网络基础，所有节点通过加密隧道互联。网络拓扑为 Full Mesh，即每个节点与其他所有节点直接连接。

- **网络段**: 10.254.0.0/24
- **接口名**: wg0
- **端口**: 51820/UDP

## 1. 安装 WireGuard

### Ubuntu/Debian

```bash
sudo apt update
sudo apt install wireguard wireguard-tools
```

### CentOS/RHEL 8+

```bash
sudo dnf install epel-release
sudo dnf install wireguard-tools
```

### CentOS/RHEL 7

```bash
sudo yum install epel-release
sudo yum install https://www.elrepo.org/elrepo-release-7.el7.elrepo.noarch.rpm
sudo yum install kmod-wireguard wireguard-tools
```

## 2. 生成密钥对

在每个节点上生成密钥对：

```bash
# 创建配置目录
sudo mkdir -p /etc/wireguard
cd /etc/wireguard

# 生成私钥和公钥
wg genkey | sudo tee privatekey | wg pubkey | sudo tee publickey

# 设置权限
sudo chmod 600 privatekey
```

记录每个节点的公钥，配置时需要交换。

## 3. Full Mesh 配置示例（3 节点）

假设有 3 个节点：

| 节点 | 公网 IP | WireGuard IP | 角色 |
|------|---------|--------------|------|
| Node A | 1.2.3.4 | 10.254.0.1 | Controller + Agent |
| Node B | 5.6.7.8 | 10.254.0.2 | Agent |
| Node C | 9.10.11.12 | 10.254.0.3 | Agent |

### Node A 配置 (/etc/wireguard/wg0.conf)

```ini
[Interface]
PrivateKey = <node_a_private_key>
Address = 10.254.0.1/24
ListenPort = 51820

# Node B
[Peer]
PublicKey = <node_b_public_key>
Endpoint = 5.6.7.8:51820
AllowedIPs = 10.254.0.2/32
PersistentKeepalive = 25

# Node C
[Peer]
PublicKey = <node_c_public_key>
Endpoint = 9.10.11.12:51820
AllowedIPs = 10.254.0.3/32
PersistentKeepalive = 25
```

### Node B 配置 (/etc/wireguard/wg0.conf)

```ini
[Interface]
PrivateKey = <node_b_private_key>
Address = 10.254.0.2/24
ListenPort = 51820

# Node A
[Peer]
PublicKey = <node_a_public_key>
Endpoint = 1.2.3.4:51820
AllowedIPs = 10.254.0.1/32
PersistentKeepalive = 25

# Node C
[Peer]
PublicKey = <node_c_public_key>
Endpoint = 9.10.11.12:51820
AllowedIPs = 10.254.0.3/32
PersistentKeepalive = 25
```

### Node C 配置 (/etc/wireguard/wg0.conf)

```ini
[Interface]
PrivateKey = <node_c_private_key>
Address = 10.254.0.3/24
ListenPort = 51820

# Node A
[Peer]
PublicKey = <node_a_public_key>
Endpoint = 1.2.3.4:51820
AllowedIPs = 10.254.0.1/32
PersistentKeepalive = 25

# Node B
[Peer]
PublicKey = <node_b_public_key>
Endpoint = 5.6.7.8:51820
AllowedIPs = 10.254.0.2/32
PersistentKeepalive = 25
```

## 4. 内核参数设置

SD-WAN 系统需要 IP 转发功能来实现中继路由。

### 临时启用

```bash
sudo sysctl -w net.ipv4.ip_forward=1
sudo sysctl -w net.ipv4.conf.all.forwarding=1
sudo sysctl -w net.ipv4.conf.wg0.forwarding=1
```

### 永久启用

创建或编辑 `/etc/sysctl.d/99-wireguard.conf`：

```bash
cat << EOF | sudo tee /etc/sysctl.d/99-wireguard.conf
# Enable IP forwarding for SD-WAN
net.ipv4.ip_forward = 1
net.ipv4.conf.all.forwarding = 1
net.ipv4.conf.default.forwarding = 1

# Disable reverse path filtering (required for asymmetric routing)
net.ipv4.conf.all.rp_filter = 0
net.ipv4.conf.default.rp_filter = 0
net.ipv4.conf.wg0.rp_filter = 0
EOF

# 应用配置
sudo sysctl -p /etc/sysctl.d/99-wireguard.conf
```

## 5. 防火墙规则

### UFW (Ubuntu)

```bash
# 允许 WireGuard 端口
sudo ufw allow 51820/udp

# 允许 WireGuard 接口转发
sudo ufw route allow in on wg0 out on wg0

# 如果运行 Controller，允许 API 端口
sudo ufw allow 8000/tcp
```

### firewalld (CentOS/RHEL)

```bash
# 允许 WireGuard 端口
sudo firewall-cmd --permanent --add-port=51820/udp

# 允许转发
sudo firewall-cmd --permanent --add-masquerade

# 如果运行 Controller
sudo firewall-cmd --permanent --add-port=8000/tcp

# 重载配置
sudo firewall-cmd --reload
```

### iptables

```bash
# 允许 WireGuard 端口
sudo iptables -A INPUT -p udp --dport 51820 -j ACCEPT

# 允许 WireGuard 接口流量
sudo iptables -A INPUT -i wg0 -j ACCEPT
sudo iptables -A FORWARD -i wg0 -j ACCEPT
sudo iptables -A FORWARD -o wg0 -j ACCEPT

# 如果运行 Controller
sudo iptables -A INPUT -p tcp --dport 8000 -j ACCEPT

# 保存规则（Ubuntu/Debian）
sudo apt install iptables-persistent
sudo netfilter-persistent save

# 保存规则（CentOS/RHEL）
sudo service iptables save
```

## 6. 启动 WireGuard

### 启动接口

```bash
# 启动
sudo wg-quick up wg0

# 查看状态
sudo wg show

# 测试连通性
ping 10.254.0.2
ping 10.254.0.3
```

### 设置开机自启

```bash
sudo systemctl enable wg-quick@wg0
```

### 停止接口

```bash
sudo wg-quick down wg0
```

## 7. 验证配置

### 检查接口状态

```bash
sudo wg show wg0
```

输出示例：

```
interface: wg0
  public key: <your_public_key>
  private key: (hidden)
  listening port: 51820

peer: <peer_public_key>
  endpoint: 5.6.7.8:51820
  allowed ips: 10.254.0.2/32
  latest handshake: 5 seconds ago
  transfer: 1.23 MiB received, 456.78 KiB sent
```

### 检查路由表

```bash
ip route show dev wg0
```

### 测试连通性

```bash
# Ping 所有节点
for ip in 10.254.0.1 10.254.0.2 10.254.0.3; do
    echo "Ping $ip:"
    ping -c 3 $ip
done
```

## 8. 故障排查

### 无法建立连接

1. 检查防火墙是否允许 UDP 51820
2. 检查公钥是否正确配置
3. 检查 Endpoint 地址是否可达

```bash
# 检查端口是否监听
sudo ss -ulnp | grep 51820

# 检查防火墙
sudo iptables -L -n | grep 51820
```

### Handshake 失败

1. 检查双方公钥配置
2. 检查时间同步（NTP）
3. 检查 NAT 穿透

```bash
# 查看 WireGuard 日志
sudo dmesg | grep wireguard
```

### 路由问题

1. 检查 IP 转发是否启用
2. 检查 AllowedIPs 配置
3. 检查 rp_filter 设置

```bash
# 检查 IP 转发
cat /proc/sys/net/ipv4/ip_forward

# 检查路由
ip route show table main
```

## 9. 安全建议

1. **保护私钥**: 确保私钥文件权限为 600
2. **定期轮换密钥**: 建议每 90 天更换一次密钥
3. **限制 AllowedIPs**: 仅允许必要的 IP 范围
4. **监控连接**: 定期检查 `wg show` 输出
5. **日志审计**: 启用系统日志记录 WireGuard 事件

## 10. 与 SD-WAN 集成

配置完成后，确保：

1. 所有节点可以互相 ping 通
2. IP 转发已启用
3. Agent 配置中的 `peer_ips` 包含所有对等节点
4. Controller 可以从所有节点访问

然后按照 README.md 中的说明启动 Controller 和 Agent。

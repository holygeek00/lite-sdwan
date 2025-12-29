#!/bin/bash
#
# Lite SD-WAN 批量部署脚本
#
# 功能：
#   - 从本地机器通过 SSH 批量部署到多个节点
#   - 自动生成所有节点的 WireGuard 配置
#   - 自动分发配置文件
#
# 用法：
#   ./batch_deploy.sh nodes.yaml
#

set -e

# 颜色定义
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_DIR="$(dirname "$SCRIPT_DIR")"
TEMP_DIR="/tmp/sdwan-deploy-$$"

log_info() { echo -e "${BLUE}[INFO]${NC} $1"; }
log_success() { echo -e "${GREEN}[SUCCESS]${NC} $1"; }
log_warn() { echo -e "${YELLOW}[WARN]${NC} $1"; }
log_error() { echo -e "${RED}[ERROR]${NC} $1"; }

#######################################
# 显示帮助
#######################################
show_help() {
    cat << EOF
Lite SD-WAN 批量部署工具

用法: $0 <nodes.yaml>

nodes.yaml 格式示例:
---
controller:
  host: 1.2.3.4
  ssh_user: root
  ssh_port: 22
  wg_ip: 10.254.0.1

agents:
  - host: 5.6.7.8
    ssh_user: root
    ssh_port: 22
    wg_ip: 10.254.0.2
  - host: 9.10.11.12
    ssh_user: root
    ssh_port: 22
    wg_ip: 10.254.0.3

EOF
    exit 0
}

#######################################
# 检查依赖
#######################################
check_dependencies() {
    log_info "检查本地依赖..."
    
    for cmd in ssh scp wg python3; do
        if ! command -v $cmd &> /dev/null; then
            log_error "缺少依赖: $cmd"
            exit 1
        fi
    done
    
    log_success "依赖检查通过"
}

#######################################
# 解析 YAML 配置（简单解析）
#######################################
parse_config() {
    local config_file="$1"
    
    if [ ! -f "$config_file" ]; then
        log_error "配置文件不存在: $config_file"
        exit 1
    fi
    
    log_info "解析配置文件: $config_file"
    
    # 使用 Python 解析 YAML
    python3 << EOF
import yaml
import json
import sys

with open('$config_file', 'r') as f:
    config = yaml.safe_load(f)

# 输出为 shell 可读格式
print(f"CONTROLLER_HOST={config['controller']['host']}")
print(f"CONTROLLER_SSH_USER={config['controller'].get('ssh_user', 'root')}")
print(f"CONTROLLER_SSH_PORT={config['controller'].get('ssh_port', 22)}")
print(f"CONTROLLER_WG_IP={config['controller']['wg_ip']}")

agents = config.get('agents', [])
print(f"AGENT_COUNT={len(agents)}")

for i, agent in enumerate(agents):
    print(f"AGENT_{i}_HOST={agent['host']}")
    print(f"AGENT_{i}_SSH_USER={agent.get('ssh_user', 'root')}")
    print(f"AGENT_{i}_SSH_PORT={agent.get('ssh_port', 22)}")
    print(f"AGENT_{i}_WG_IP={agent['wg_ip']}")
EOF
}

#######################################
# 生成节点密钥
#######################################
generate_all_keys() {
    log_info "生成所有节点的 WireGuard 密钥..."
    
    mkdir -p "$TEMP_DIR/keys"
    
    # Controller 密钥
    wg genkey | tee "$TEMP_DIR/keys/controller_private" | wg pubkey > "$TEMP_DIR/keys/controller_public"
    
    # Agent 密钥
    for i in $(seq 0 $((AGENT_COUNT - 1))); do
        wg genkey | tee "$TEMP_DIR/keys/agent_${i}_private" | wg pubkey > "$TEMP_DIR/keys/agent_${i}_public"
    done
    
    log_success "密钥生成完成"
}

#######################################
# 生成 WireGuard 配置
#######################################
generate_wg_configs() {
    log_info "生成 WireGuard 配置文件..."
    
    mkdir -p "$TEMP_DIR/configs"
    
    local controller_pubkey=$(cat "$TEMP_DIR/keys/controller_public")
    local controller_privkey=$(cat "$TEMP_DIR/keys/controller_private")
    
    # Controller 的 WireGuard 配置
    cat > "$TEMP_DIR/configs/controller_wg0.conf" << EOF
[Interface]
PrivateKey = $controller_privkey
Address = $CONTROLLER_WG_IP/24
ListenPort = 51820
EOF
    
    # 添加所有 Agent 作为 Peer
    for i in $(seq 0 $((AGENT_COUNT - 1))); do
        local agent_host=$(eval echo \$AGENT_${i}_HOST)
        local agent_wg_ip=$(eval echo \$AGENT_${i}_WG_IP)
        local agent_pubkey=$(cat "$TEMP_DIR/keys/agent_${i}_public")
        
        cat >> "$TEMP_DIR/configs/controller_wg0.conf" << EOF

[Peer]
PublicKey = $agent_pubkey
Endpoint = $agent_host:51820
AllowedIPs = $agent_wg_ip/32
PersistentKeepalive = 25
EOF
    done
    
    # 每个 Agent 的 WireGuard 配置
    for i in $(seq 0 $((AGENT_COUNT - 1))); do
        local agent_wg_ip=$(eval echo \$AGENT_${i}_WG_IP)
        local agent_privkey=$(cat "$TEMP_DIR/keys/agent_${i}_private")
        
        cat > "$TEMP_DIR/configs/agent_${i}_wg0.conf" << EOF
[Interface]
PrivateKey = $agent_privkey
Address = $agent_wg_ip/24
ListenPort = 51820

[Peer]
PublicKey = $controller_pubkey
Endpoint = $CONTROLLER_HOST:51820
AllowedIPs = $CONTROLLER_WG_IP/32
PersistentKeepalive = 25
EOF
        
        # 添加其他 Agent 作为 Peer
        for j in $(seq 0 $((AGENT_COUNT - 1))); do
            if [ $i -ne $j ]; then
                local other_host=$(eval echo \$AGENT_${j}_HOST)
                local other_wg_ip=$(eval echo \$AGENT_${j}_WG_IP)
                local other_pubkey=$(cat "$TEMP_DIR/keys/agent_${j}_public")
                
                cat >> "$TEMP_DIR/configs/agent_${i}_wg0.conf" << EOF

[Peer]
PublicKey = $other_pubkey
Endpoint = $other_host:51820
AllowedIPs = $other_wg_ip/32
PersistentKeepalive = 25
EOF
            fi
        done
    done
    
    log_success "WireGuard 配置生成完成"
}


#######################################
# 生成 SD-WAN 配置
#######################################
generate_sdwan_configs() {
    log_info "生成 SD-WAN 配置文件..."
    
    # Controller 配置
    cat > "$TEMP_DIR/configs/controller_config.yaml" << EOF
server:
  listen_address: "0.0.0.0"
  port: 8000

algorithm:
  penalty_factor: 100
  hysteresis: 0.15

topology:
  stale_threshold: 60

logging:
  level: "INFO"
EOF
    
    # Controller 的 Agent 配置
    local peer_ips=""
    for i in $(seq 0 $((AGENT_COUNT - 1))); do
        local agent_wg_ip=$(eval echo \$AGENT_${i}_WG_IP)
        peer_ips="$peer_ips    - \"$agent_wg_ip\"\n"
    done
    
    cat > "$TEMP_DIR/configs/controller_agent_config.yaml" << EOF
agent_id: "$CONTROLLER_WG_IP"

controller:
  url: "http://$CONTROLLER_WG_IP:8000"
  timeout: 5

probe:
  interval: 5
  timeout: 2
  window_size: 10

sync:
  interval: 10
  retry_attempts: 3
  retry_backoff: [1, 2, 4]

network:
  wg_interface: "wg0"
  subnet: "10.254.0.0/24"
  peer_ips:
$(echo -e "$peer_ips")
EOF
    
    # 每个 Agent 的配置
    for i in $(seq 0 $((AGENT_COUNT - 1))); do
        local agent_wg_ip=$(eval echo \$AGENT_${i}_WG_IP)
        
        # 构建 peer_ips 列表（包括 Controller 和其他 Agent）
        local peer_ips="    - \"$CONTROLLER_WG_IP\"\n"
        for j in $(seq 0 $((AGENT_COUNT - 1))); do
            if [ $i -ne $j ]; then
                local other_wg_ip=$(eval echo \$AGENT_${j}_WG_IP)
                peer_ips="$peer_ips    - \"$other_wg_ip\"\n"
            fi
        done
        
        cat > "$TEMP_DIR/configs/agent_${i}_config.yaml" << EOF
agent_id: "$agent_wg_ip"

controller:
  url: "http://$CONTROLLER_WG_IP:8000"
  timeout: 5

probe:
  interval: 5
  timeout: 2
  window_size: 10

sync:
  interval: 10
  retry_attempts: 3
  retry_backoff: [1, 2, 4]

network:
  wg_interface: "wg0"
  subnet: "10.254.0.0/24"
  peer_ips:
$(echo -e "$peer_ips")
EOF
    done
    
    log_success "SD-WAN 配置生成完成"
}

#######################################
# 部署到单个节点
#######################################
deploy_to_node() {
    local host="$1"
    local ssh_user="$2"
    local ssh_port="$3"
    local role="$4"
    local wg_config="$5"
    local agent_config="$6"
    local controller_config="${7:-}"
    
    log_info "部署到 $host ($role)..."
    
    local ssh_opts="-o StrictHostKeyChecking=no -o ConnectTimeout=10 -p $ssh_port"
    local ssh_cmd="ssh $ssh_opts $ssh_user@$host"
    local scp_cmd="scp $ssh_opts"
    
    # 创建远程目录
    $ssh_cmd "mkdir -p /opt/sdwan /etc/sdwan /etc/wireguard /var/log/sdwan"
    
    # 复制项目文件
    $scp_cmd -r "$PROJECT_DIR/agent" "$ssh_user@$host:/opt/sdwan/"
    $scp_cmd -r "$PROJECT_DIR/controller" "$ssh_user@$host:/opt/sdwan/"
    $scp_cmd -r "$PROJECT_DIR/config" "$ssh_user@$host:/opt/sdwan/"
    $scp_cmd "$PROJECT_DIR/models.py" "$ssh_user@$host:/opt/sdwan/"
    $scp_cmd "$PROJECT_DIR/requirements.txt" "$ssh_user@$host:/opt/sdwan/"
    
    # 复制配置文件
    $scp_cmd "$wg_config" "$ssh_user@$host:/etc/wireguard/wg0.conf"
    $scp_cmd "$agent_config" "$ssh_user@$host:/etc/sdwan/agent_config.yaml"
    
    if [ -n "$controller_config" ]; then
        $scp_cmd "$controller_config" "$ssh_user@$host:/etc/sdwan/controller_config.yaml"
    fi
    
    # 远程执行安装脚本
    $ssh_cmd << 'REMOTE_SCRIPT'
set -e

# 检测操作系统并安装依赖
if [ -f /etc/os-release ]; then
    . /etc/os-release
    case $ID in
        ubuntu|debian)
            apt-get update -qq
            apt-get install -y -qq python3 python3-pip python3-venv wireguard wireguard-tools
            ;;
        centos|rhel|rocky|almalinux)
            dnf install -y epel-release 2>/dev/null || yum install -y epel-release
            dnf install -y python3 python3-pip wireguard-tools 2>/dev/null || yum install -y python3 python3-pip wireguard-tools
            ;;
    esac
fi

# 创建虚拟环境并安装依赖
python3 -m venv /opt/sdwan/venv
source /opt/sdwan/venv/bin/activate
pip install --upgrade pip -q
pip install -r /opt/sdwan/requirements.txt -q
deactivate

# 配置内核参数
cat > /etc/sysctl.d/99-sdwan.conf << EOF
net.ipv4.ip_forward = 1
net.ipv4.conf.all.forwarding = 1
net.ipv4.conf.all.rp_filter = 0
net.ipv4.conf.default.rp_filter = 0
EOF
sysctl -p /etc/sysctl.d/99-sdwan.conf > /dev/null 2>&1

# 设置 WireGuard 配置权限
chmod 600 /etc/wireguard/wg0.conf

echo "远程安装完成"
REMOTE_SCRIPT
    
    log_success "文件部署完成: $host"
}

#######################################
# 安装 systemd 服务
#######################################
install_services_on_node() {
    local host="$1"
    local ssh_user="$2"
    local ssh_port="$3"
    local role="$4"
    
    log_info "安装服务到 $host..."
    
    local ssh_opts="-o StrictHostKeyChecking=no -o ConnectTimeout=10 -p $ssh_port"
    local ssh_cmd="ssh $ssh_opts $ssh_user@$host"
    
    # Agent 服务
    $ssh_cmd << 'AGENT_SERVICE'
cat > /etc/systemd/system/sdwan-agent.service << EOF
[Unit]
Description=SD-WAN Agent Service
After=network.target wg-quick@wg0.service
Wants=wg-quick@wg0.service

[Service]
Type=simple
ExecStart=/opt/sdwan/venv/bin/python -m agent.main /etc/sdwan/agent_config.yaml
WorkingDirectory=/opt/sdwan
Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target
EOF
AGENT_SERVICE
    
    if [ "$role" = "controller" ]; then
        $ssh_cmd << 'CONTROLLER_SERVICE'
# 创建 sdwan 用户
id -u sdwan &>/dev/null || useradd -r -s /bin/false sdwan
chown -R sdwan:sdwan /opt/sdwan
chown -R sdwan:sdwan /var/log/sdwan

cat > /etc/systemd/system/sdwan-controller.service << EOF
[Unit]
Description=SD-WAN Controller Service
After=network.target

[Service]
Type=simple
User=sdwan
Group=sdwan
ExecStart=/opt/sdwan/venv/bin/python -m uvicorn controller.api:app --host 0.0.0.0 --port 8000
WorkingDirectory=/opt/sdwan
Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target
EOF
CONTROLLER_SERVICE
    fi
    
    log_success "服务安装完成: $host"
}

#######################################
# 启动所有服务
#######################################
start_services_on_node() {
    local host="$1"
    local ssh_user="$2"
    local ssh_port="$3"
    local role="$4"
    
    log_info "启动服务: $host..."
    
    local ssh_opts="-o StrictHostKeyChecking=no -o ConnectTimeout=10 -p $ssh_port"
    local ssh_cmd="ssh $ssh_opts $ssh_user@$host"
    
    $ssh_cmd << REMOTE_START
systemctl daemon-reload
systemctl enable wg-quick@wg0
systemctl start wg-quick@wg0 || wg-quick up wg0

if [ "$role" = "controller" ]; then
    systemctl enable sdwan-controller
    systemctl start sdwan-controller
    sleep 2
fi

systemctl enable sdwan-agent
systemctl start sdwan-agent
REMOTE_START
    
    log_success "服务启动完成: $host"
}

#######################################
# 主函数
#######################################
main() {
    local config_file="${1:-}"
    
    if [ -z "$config_file" ] || [ "$config_file" = "--help" ] || [ "$config_file" = "-h" ]; then
        show_help
    fi
    
    echo ""
    echo -e "${BLUE}╔════════════════════════════════════════╗${NC}"
    echo -e "${BLUE}║   Lite SD-WAN 批量部署工具 v1.0       ║${NC}"
    echo -e "${BLUE}╚════════════════════════════════════════╝${NC}"
    echo ""
    
    # 检查依赖
    check_dependencies
    
    # 解析配置
    eval "$(parse_config "$config_file")"
    
    # 创建临时目录
    mkdir -p "$TEMP_DIR"
    trap "rm -rf $TEMP_DIR" EXIT
    
    # 生成密钥和配置
    generate_all_keys
    generate_wg_configs
    generate_sdwan_configs
    
    echo ""
    log_info "开始部署到 $((AGENT_COUNT + 1)) 个节点..."
    echo ""
    
    # 部署 Controller
    deploy_to_node "$CONTROLLER_HOST" "$CONTROLLER_SSH_USER" "$CONTROLLER_SSH_PORT" "controller" \
        "$TEMP_DIR/configs/controller_wg0.conf" \
        "$TEMP_DIR/configs/controller_agent_config.yaml" \
        "$TEMP_DIR/configs/controller_config.yaml"
    
    install_services_on_node "$CONTROLLER_HOST" "$CONTROLLER_SSH_USER" "$CONTROLLER_SSH_PORT" "controller"
    
    # 部署 Agents
    for i in $(seq 0 $((AGENT_COUNT - 1))); do
        local agent_host=$(eval echo \$AGENT_${i}_HOST)
        local agent_ssh_user=$(eval echo \$AGENT_${i}_SSH_USER)
        local agent_ssh_port=$(eval echo \$AGENT_${i}_SSH_PORT)
        
        deploy_to_node "$agent_host" "$agent_ssh_user" "$agent_ssh_port" "agent" \
            "$TEMP_DIR/configs/agent_${i}_wg0.conf" \
            "$TEMP_DIR/configs/agent_${i}_config.yaml"
        
        install_services_on_node "$agent_host" "$agent_ssh_user" "$agent_ssh_port" "agent"
    done
    
    echo ""
    log_info "启动所有服务..."
    
    # 先启动 Controller
    start_services_on_node "$CONTROLLER_HOST" "$CONTROLLER_SSH_USER" "$CONTROLLER_SSH_PORT" "controller"
    
    # 再启动 Agents
    for i in $(seq 0 $((AGENT_COUNT - 1))); do
        local agent_host=$(eval echo \$AGENT_${i}_HOST)
        local agent_ssh_user=$(eval echo \$AGENT_${i}_SSH_USER)
        local agent_ssh_port=$(eval echo \$AGENT_${i}_SSH_PORT)
        
        start_services_on_node "$agent_host" "$agent_ssh_user" "$agent_ssh_port" "agent"
    done
    
    echo ""
    echo -e "${GREEN}========================================${NC}"
    echo -e "${GREEN}       批量部署完成！${NC}"
    echo -e "${GREEN}========================================${NC}"
    echo ""
    echo "Controller: $CONTROLLER_HOST ($CONTROLLER_WG_IP)"
    for i in $(seq 0 $((AGENT_COUNT - 1))); do
        local agent_host=$(eval echo \$AGENT_${i}_HOST)
        local agent_wg_ip=$(eval echo \$AGENT_${i}_WG_IP)
        echo "Agent $((i+1)): $agent_host ($agent_wg_ip)"
    done
    echo ""
    echo "验证命令:"
    echo "  curl http://$CONTROLLER_HOST:8000/health"
    echo ""
}

main "$@"

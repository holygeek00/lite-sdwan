#!/bin/bash
#
# Lite SD-WAN 批量部署脚本 (Go 版本)
#
# 功能：
#   - 从本地机器通过 SSH 批量部署到多个节点
#   - 自动生成所有节点的 WireGuard 配置
#   - 自动分发二进制文件和配置
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
CYAN='\033[0;36m'
BOLD='\033[1m'
NC='\033[0m'

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_DIR="$(dirname "$SCRIPT_DIR")"
TEMP_DIR="/tmp/sdwan-deploy-$$"
GITHUB_REPO="holygeek00/lite-sdwan"

log_info() { echo -e "${BLUE}[INFO]${NC} $1"; }
log_success() { echo -e "${GREEN}[✓]${NC} $1"; }
log_warn() { echo -e "${YELLOW}[!]${NC} $1"; }
log_error() { echo -e "${RED}[✗]${NC} $1"; exit 1; }
log_step() { echo -e "${BOLD}${CYAN}==>${NC} ${BOLD}$1${NC}"; }

show_help() {
    cat << EOF
Lite SD-WAN 批量部署工具 (Go 版本)

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

check_dependencies() {
    log_step "检查本地依赖..."
    
    for cmd in ssh scp wg; do
        if ! command -v $cmd &> /dev/null; then
            log_error "缺少依赖: $cmd"
        fi
    done
    
    # 检查 Python 或 yq 用于解析 YAML
    if ! command -v python3 &> /dev/null && ! command -v yq &> /dev/null; then
        log_error "需要 python3 或 yq 来解析 YAML 配置"
    fi
    
    log_success "依赖检查通过"
}

parse_config() {
    local config_file="$1"
    
    if [ ! -f "$config_file" ]; then
        log_error "配置文件不存在: $config_file"
    fi
    
    log_info "解析配置文件: $config_file"
    
    # 使用 Python 解析 YAML
    python3 << EOF
import yaml
import sys

with open('$config_file', 'r') as f:
    config = yaml.safe_load(f)

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

generate_all_keys() {
    log_step "生成所有节点的 WireGuard 密钥..."
    
    mkdir -p "$TEMP_DIR/keys"
    
    # Controller 密钥
    wg genkey | tee "$TEMP_DIR/keys/controller_private" | wg pubkey > "$TEMP_DIR/keys/controller_public"
    
    # Agent 密钥
    for i in $(seq 0 $((AGENT_COUNT - 1))); do
        wg genkey | tee "$TEMP_DIR/keys/agent_${i}_private" | wg pubkey > "$TEMP_DIR/keys/agent_${i}_public"
    done
    
    log_success "密钥生成完成"
}

generate_wg_configs() {
    log_step "生成 WireGuard 配置文件..."
    
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

generate_sdwan_configs() {
    log_step "生成 SD-WAN 配置文件..."
    
    # Controller 配置
    cat > "$TEMP_DIR/configs/controller_config.yaml" << EOF
server:
  listen_address: "0.0.0.0"
  port: 8000

algorithm:
  penalty_factor: 100
  hysteresis: 0.15

topology:
  stale_threshold: 60s

logging:
  level: "INFO"
EOF
    
    # Controller 的 Agent 配置
    cat > "$TEMP_DIR/configs/controller_agent_config.yaml" << EOF
agent_id: "$CONTROLLER_WG_IP"

controller:
  url: "http://$CONTROLLER_WG_IP:8000"
  timeout: 5s

probe:
  interval: 5s
  timeout: 2s
  window_size: 10

sync:
  interval: 10s
  retry_attempts: 3
  retry_backoff: [1, 2, 4]

network:
  wg_interface: "wg0"
  subnet: "10.254.0.0/24"
  peer_ips:
EOF
    
    for i in $(seq 0 $((AGENT_COUNT - 1))); do
        local agent_wg_ip=$(eval echo \$AGENT_${i}_WG_IP)
        echo "    - \"$agent_wg_ip\"" >> "$TEMP_DIR/configs/controller_agent_config.yaml"
    done
    
    # 每个 Agent 的配置
    for i in $(seq 0 $((AGENT_COUNT - 1))); do
        local agent_wg_ip=$(eval echo \$AGENT_${i}_WG_IP)
        
        cat > "$TEMP_DIR/configs/agent_${i}_config.yaml" << EOF
agent_id: "$agent_wg_ip"

controller:
  url: "http://$CONTROLLER_WG_IP:8000"
  timeout: 5s

probe:
  interval: 5s
  timeout: 2s
  window_size: 10

sync:
  interval: 10s
  retry_attempts: 3
  retry_backoff: [1, 2, 4]

network:
  wg_interface: "wg0"
  subnet: "10.254.0.0/24"
  peer_ips:
    - "$CONTROLLER_WG_IP"
EOF
        
        for j in $(seq 0 $((AGENT_COUNT - 1))); do
            if [ $i -ne $j ]; then
                local other_wg_ip=$(eval echo \$AGENT_${j}_WG_IP)
                echo "    - \"$other_wg_ip\"" >> "$TEMP_DIR/configs/agent_${i}_config.yaml"
            fi
        done
    done
    
    log_success "SD-WAN 配置生成完成"
}

deploy_to_node() {
    local host="$1"
    local ssh_user="$2"
    local ssh_port="$3"
    local role="$4"
    local wg_config="$5"
    local agent_config="$6"
    local controller_config="${7:-}"
    
    log_info "部署到 $host ($role)..."
    
    local ssh_opts="-o StrictHostKeyChecking=no -o ConnectTimeout=30 -p $ssh_port"
    local ssh_cmd="ssh $ssh_opts $ssh_user@$host"
    local scp_cmd="scp -o StrictHostKeyChecking=no -P $ssh_port"
    
    # 检测远程系统架构
    local remote_arch=$($ssh_cmd "uname -m")
    local arch_suffix=""
    case $remote_arch in
        x86_64)  arch_suffix="linux-amd64" ;;
        aarch64) arch_suffix="linux-arm64" ;;
        armv7l)  arch_suffix="linux-armv7" ;;
        *) log_error "不支持的架构: $remote_arch" ;;
    esac
    
    # 远程安装依赖和下载二进制
    $ssh_cmd << REMOTE_INSTALL
set -e

# 检测操作系统
if [ -f /etc/os-release ]; then
    . /etc/os-release
    OS=\$ID
else
    OS="unknown"
fi

# 安装依赖
case \$OS in
    ubuntu|debian)
        apt-get update -qq >/dev/null 2>&1
        apt-get install -y -qq curl wget wireguard wireguard-tools >/dev/null 2>&1
        ;;
    centos|rhel|rocky|almalinux)
        yum install -y epel-release >/dev/null 2>&1 || true
        yum install -y curl wget wireguard-tools >/dev/null 2>&1
        ;;
    fedora)
        dnf install -y curl wget wireguard-tools >/dev/null 2>&1
        ;;
esac

# 创建目录
mkdir -p /usr/local/bin /etc/sdwan /etc/wireguard

# 下载二进制文件
echo "下载二进制文件..."
curl -sLf "https://github.com/${GITHUB_REPO}/releases/latest/download/sdwan-controller-${arch_suffix}" -o /usr/local/bin/sdwan-controller || true
curl -sLf "https://github.com/${GITHUB_REPO}/releases/latest/download/sdwan-agent-${arch_suffix}" -o /usr/local/bin/sdwan-agent || true
chmod +x /usr/local/bin/sdwan-controller /usr/local/bin/sdwan-agent 2>/dev/null || true

# 配置内核参数
cat > /etc/sysctl.d/99-sdwan.conf << 'SYSCTL'
net.ipv4.ip_forward = 1
net.ipv4.conf.all.forwarding = 1
net.ipv4.conf.all.rp_filter = 0
net.ipv4.conf.default.rp_filter = 0
SYSCTL
sysctl -p /etc/sysctl.d/99-sdwan.conf > /dev/null 2>&1

echo "远程安装完成"
REMOTE_INSTALL
    
    # 复制配置文件
    $scp_cmd "$wg_config" "$ssh_user@$host:/etc/wireguard/wg0.conf"
    $scp_cmd "$agent_config" "$ssh_user@$host:/etc/sdwan/agent_config.yaml"
    
    if [ -n "$controller_config" ]; then
        $scp_cmd "$controller_config" "$ssh_user@$host:/etc/sdwan/controller_config.yaml"
    fi
    
    # 设置权限
    $ssh_cmd "chmod 600 /etc/wireguard/wg0.conf"
    
    log_success "部署完成: $host"
}

install_services_on_node() {
    local host="$1"
    local ssh_user="$2"
    local ssh_port="$3"
    local role="$4"
    
    log_info "安装服务到 $host..."
    
    local ssh_opts="-o StrictHostKeyChecking=no -o ConnectTimeout=30 -p $ssh_port"
    local ssh_cmd="ssh $ssh_opts $ssh_user@$host"
    
    # Agent 服务
    $ssh_cmd << 'AGENT_SERVICE'
cat > /etc/systemd/system/sdwan-agent.service << 'EOF'
[Unit]
Description=SD-WAN Agent
After=network.target wg-quick@wg0.service
Wants=wg-quick@wg0.service

[Service]
Type=simple
ExecStart=/usr/local/bin/sdwan-agent -config /etc/sdwan/agent_config.yaml
Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target
EOF
AGENT_SERVICE
    
    if [ "$role" = "controller" ]; then
        $ssh_cmd << 'CONTROLLER_SERVICE'
cat > /etc/systemd/system/sdwan-controller.service << 'EOF'
[Unit]
Description=SD-WAN Controller
After=network.target

[Service]
Type=simple
ExecStart=/usr/local/bin/sdwan-controller -config /etc/sdwan/controller_config.yaml
Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target
EOF
CONTROLLER_SERVICE
    fi
    
    log_success "服务安装完成: $host"
}

start_services_on_node() {
    local host="$1"
    local ssh_user="$2"
    local ssh_port="$3"
    local role="$4"
    
    log_info "启动服务: $host..."
    
    local ssh_opts="-o StrictHostKeyChecking=no -o ConnectTimeout=30 -p $ssh_port"
    local ssh_cmd="ssh $ssh_opts $ssh_user@$host"
    
    $ssh_cmd << REMOTE_START
set -e
systemctl daemon-reload
systemctl enable wg-quick@wg0 2>/dev/null || true
systemctl start wg-quick@wg0 2>/dev/null || wg-quick up wg0 2>/dev/null || true

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

main() {
    local config_file="${1:-}"
    
    if [ -z "$config_file" ] || [ "$config_file" = "--help" ] || [ "$config_file" = "-h" ]; then
        show_help
    fi
    
    echo ""
    echo -e "${CYAN}╔════════════════════════════════════════╗${NC}"
    echo -e "${CYAN}║   Lite SD-WAN 批量部署 (Go 版本)      ║${NC}"
    echo -e "${CYAN}╚════════════════════════════════════════╝${NC}"
    echo ""
    
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
    log_step "启动所有服务..."
    
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
    echo -e "${GREEN}╔════════════════════════════════════════╗${NC}"
    echo -e "${GREEN}║         批量部署完成！                 ║${NC}"
    echo -e "${GREEN}╚════════════════════════════════════════╝${NC}"
    echo ""
    echo -e "${CYAN}节点信息:${NC}"
    echo "  Controller: $CONTROLLER_HOST ($CONTROLLER_WG_IP)"
    for i in $(seq 0 $((AGENT_COUNT - 1))); do
        local agent_host=$(eval echo \$AGENT_${i}_HOST)
        local agent_wg_ip=$(eval echo \$AGENT_${i}_WG_IP)
        echo "  Agent $((i+1)): $agent_host ($agent_wg_ip)"
    done
    echo ""
    echo -e "${CYAN}验证命令:${NC}"
    echo "  curl http://$CONTROLLER_HOST:8000/health"
    echo ""
}

main "$@"

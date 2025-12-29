#!/bin/bash
#
# Lite SD-WAN 一键部署脚本 (Go 版本)
#
# 功能：
#   - 自动检测系统环境
#   - 下载或编译二进制文件
#   - 安装 WireGuard
#   - 生成配置文件
#   - 配置 systemd 服务
#

set -e

# 颜色定义
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

# 默认配置
SDWAN_BIN="/usr/local/bin"
SDWAN_CONFIG="/etc/sdwan"
WG_INTERFACE="wg0"
WG_PORT=51820
WG_SUBNET="10.254.0.0/24"
CONTROLLER_PORT=8000

# 脚本所在目录
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_DIR="$(dirname "$SCRIPT_DIR")"

log_info() { echo -e "${BLUE}[INFO]${NC} $1"; }
log_success() { echo -e "${GREEN}[SUCCESS]${NC} $1"; }
log_warn() { echo -e "${YELLOW}[WARN]${NC} $1"; }
log_error() { echo -e "${RED}[ERROR]${NC} $1"; }

check_root() {
    if [ "$EUID" -ne 0 ]; then
        log_error "请使用 root 权限运行此脚本"
        exit 1
    fi
}

detect_os() {
    if [ -f /etc/os-release ]; then
        . /etc/os-release
        OS=$ID
        OS_VERSION=$VERSION_ID
    else
        OS="unknown"
    fi
    log_info "检测到操作系统: $OS $OS_VERSION"
}

detect_arch() {
    ARCH=$(uname -m)
    case $ARCH in
        x86_64) ARCH="amd64" ;;
        aarch64) ARCH="arm64" ;;
        *) log_error "不支持的架构: $ARCH"; exit 1 ;;
    esac
    log_info "检测到架构: $ARCH"
}

install_wireguard() {
    log_info "安装 WireGuard..."
    
    case $OS in
        ubuntu|debian)
            apt-get update -qq
            apt-get install -y -qq wireguard wireguard-tools
            ;;
        centos|rhel|rocky|almalinux)
            dnf install -y epel-release 2>/dev/null || yum install -y epel-release
            dnf install -y wireguard-tools 2>/dev/null || yum install -y wireguard-tools
            ;;
        fedora)
            dnf install -y wireguard-tools
            ;;
        arch|manjaro)
            pacman -Sy --noconfirm wireguard-tools
            ;;
        *)
            log_warn "请手动安装 wireguard-tools"
            ;;
    esac
    
    log_success "WireGuard 安装完成"
}

build_binaries() {
    log_info "编译二进制文件..."
    
    # 检查 Go 是否安装
    if ! command -v go &> /dev/null; then
        log_info "安装 Go..."
        case $OS in
            ubuntu|debian)
                apt-get install -y -qq golang
                ;;
            centos|rhel|rocky|almalinux|fedora)
                dnf install -y golang 2>/dev/null || yum install -y golang
                ;;
            arch|manjaro)
                pacman -Sy --noconfirm go
                ;;
        esac
    fi
    
    cd "$PROJECT_DIR"
    
    # 下载依赖
    go mod download
    
    # 编译
    go build -o "$SDWAN_BIN/sdwan-controller" ./cmd/controller
    go build -o "$SDWAN_BIN/sdwan-agent" ./cmd/agent
    
    log_success "编译完成"
}

generate_wg_keys() {
    log_info "生成 WireGuard 密钥..."
    
    mkdir -p /etc/wireguard
    
    if [ ! -f /etc/wireguard/privatekey ]; then
        wg genkey | tee /etc/wireguard/privatekey | wg pubkey > /etc/wireguard/publickey
        chmod 600 /etc/wireguard/privatekey
        log_success "密钥生成完成"
    else
        log_info "密钥已存在"
    fi
}

configure_kernel() {
    log_info "配置内核参数..."
    
    cat > /etc/sysctl.d/99-sdwan.conf << EOF
net.ipv4.ip_forward = 1
net.ipv4.conf.all.forwarding = 1
net.ipv4.conf.all.rp_filter = 0
net.ipv4.conf.default.rp_filter = 0
EOF
    
    sysctl -p /etc/sysctl.d/99-sdwan.conf > /dev/null 2>&1
    log_success "内核参数配置完成"
}

get_node_info() {
    echo ""
    echo -e "${BLUE}========================================${NC}"
    echo -e "${BLUE}       SD-WAN 节点配置向导${NC}"
    echo -e "${BLUE}========================================${NC}"
    echo ""
    
    echo "请选择此节点的角色:"
    echo "  1) Controller + Agent (中心节点)"
    echo "  2) Agent (普通节点)"
    read -p "请输入选项 [1/2]: " role_choice
    
    case $role_choice in
        1) NODE_ROLE="controller" ;;
        *) NODE_ROLE="agent" ;;
    esac
    
    read -p "请输入本机 WireGuard IP (例如 10.254.0.1): " NODE_WG_IP
    
    DEFAULT_PUBLIC_IP=$(curl -s ifconfig.me 2>/dev/null || echo "")
    read -p "请输入本机公网 IP [$DEFAULT_PUBLIC_IP]: " NODE_PUBLIC_IP
    NODE_PUBLIC_IP=${NODE_PUBLIC_IP:-$DEFAULT_PUBLIC_IP}
    
    if [ "$NODE_ROLE" = "agent" ]; then
        read -p "请输入 Controller 地址 (例如 http://10.254.0.1:8000): " CONTROLLER_URL
    else
        CONTROLLER_URL="http://${NODE_WG_IP}:${CONTROLLER_PORT}"
    fi
    
    echo ""
    echo "请输入所有对等节点信息（不包括本机）"
    echo "格式: WireGuard_IP,公网IP,公钥"
    echo "输入空行结束"
    
    PEERS=()
    while true; do
        read -p "对等节点: " peer_info
        [ -z "$peer_info" ] && break
        PEERS+=("$peer_info")
    done
    
    echo ""
    echo -e "${GREEN}配置确认:${NC}"
    echo "  角色: $NODE_ROLE"
    echo "  WireGuard IP: $NODE_WG_IP"
    echo "  公网 IP: $NODE_PUBLIC_IP"
    echo "  Controller: $CONTROLLER_URL"
    echo "  对等节点数: ${#PEERS[@]}"
    
    read -p "确认? [Y/n]: " confirm
    [[ "$confirm" =~ ^[Nn] ]] && exit 1
}

generate_wg_config() {
    log_info "生成 WireGuard 配置..."
    
    local private_key=$(cat /etc/wireguard/privatekey)
    
    cat > /etc/wireguard/$WG_INTERFACE.conf << EOF
[Interface]
PrivateKey = $private_key
Address = $NODE_WG_IP/24
ListenPort = $WG_PORT
EOF
    
    for peer in "${PEERS[@]}"; do
        IFS=',' read -r peer_wg_ip peer_public_ip peer_pubkey <<< "$peer"
        cat >> /etc/wireguard/$WG_INTERFACE.conf << EOF

[Peer]
PublicKey = $peer_pubkey
Endpoint = $peer_public_ip:$WG_PORT
AllowedIPs = $peer_wg_ip/32
PersistentKeepalive = 25
EOF
    done
    
    chmod 600 /etc/wireguard/$WG_INTERFACE.conf
    log_success "WireGuard 配置完成"
}

generate_sdwan_config() {
    log_info "生成 SD-WAN 配置..."
    
    mkdir -p "$SDWAN_CONFIG"
    
    # Agent 配置
    local peer_ips=""
    for peer in "${PEERS[@]}"; do
        IFS=',' read -r peer_wg_ip _ _ <<< "$peer"
        peer_ips="$peer_ips    - \"$peer_wg_ip\"\n"
    done
    
    cat > "$SDWAN_CONFIG/agent_config.yaml" << EOF
agent_id: "$NODE_WG_IP"

controller:
  url: "$CONTROLLER_URL"
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
  wg_interface: "$WG_INTERFACE"
  subnet: "$WG_SUBNET"
  peer_ips:
$(echo -e "$peer_ips")
EOF
    
    if [ "$NODE_ROLE" = "controller" ]; then
        cat > "$SDWAN_CONFIG/controller_config.yaml" << EOF
server:
  listen_address: "0.0.0.0"
  port: $CONTROLLER_PORT

algorithm:
  penalty_factor: 100
  hysteresis: 0.15

topology:
  stale_threshold: 60s

logging:
  level: "INFO"
EOF
    fi
    
    log_success "SD-WAN 配置完成"
}

install_services() {
    log_info "安装 systemd 服务..."
    
    cp "$PROJECT_DIR/systemd/sdwan-agent.service" /etc/systemd/system/
    
    if [ "$NODE_ROLE" = "controller" ]; then
        cp "$PROJECT_DIR/systemd/sdwan-controller.service" /etc/systemd/system/
    fi
    
    systemctl daemon-reload
    log_success "服务安装完成"
}

start_services() {
    log_info "启动服务..."
    
    systemctl enable wg-quick@$WG_INTERFACE
    systemctl start wg-quick@$WG_INTERFACE || wg-quick up $WG_INTERFACE
    
    if [ "$NODE_ROLE" = "controller" ]; then
        systemctl enable sdwan-controller
        systemctl start sdwan-controller
        sleep 2
    fi
    
    systemctl enable sdwan-agent
    systemctl start sdwan-agent
    
    log_success "服务启动完成"
}

show_result() {
    local public_key=$(cat /etc/wireguard/publickey)
    
    echo ""
    echo -e "${GREEN}========================================${NC}"
    echo -e "${GREEN}       部署完成！${NC}"
    echo -e "${GREEN}========================================${NC}"
    echo ""
    echo "本机信息:"
    echo "  角色: $NODE_ROLE"
    echo "  WireGuard IP: $NODE_WG_IP"
    echo "  公钥: $public_key"
    echo ""
    echo "常用命令:"
    echo "  wg show"
    echo "  journalctl -u sdwan-agent -f"
    if [ "$NODE_ROLE" = "controller" ]; then
        echo "  journalctl -u sdwan-controller -f"
        echo "  curl http://localhost:$CONTROLLER_PORT/health"
    fi
    echo ""
    echo -e "${YELLOW}分享给其他节点:${NC}"
    echo "  $NODE_WG_IP,$NODE_PUBLIC_IP,$public_key"
}

main() {
    echo ""
    echo -e "${BLUE}╔════════════════════════════════════════╗${NC}"
    echo -e "${BLUE}║   Lite SD-WAN 一键部署 (Go 版本)      ║${NC}"
    echo -e "${BLUE}╚════════════════════════════════════════╝${NC}"
    echo ""
    
    check_root
    detect_os
    detect_arch
    install_wireguard
    build_binaries
    generate_wg_keys
    get_node_info
    configure_kernel
    generate_wg_config
    generate_sdwan_config
    install_services
    start_services
    show_result
}

main "$@"

#!/bin/bash
#
# Lite SD-WAN 一键安装脚本
#
# 用法:
#   curl -sSL https://raw.githubusercontent.com/holygeek00/lite-sdwan/main/deploy/install.sh | sudo bash
#
# 或者:
#   wget -qO- https://raw.githubusercontent.com/holygeek00/lite-sdwan/main/deploy/install.sh | sudo bash
#

set -e

# ============== 配置 ==============
GITHUB_REPO="holygeek00/lite-sdwan"
INSTALL_DIR="/usr/local/bin"
CONFIG_DIR="/etc/sdwan"
SYSTEMD_DIR="/etc/systemd/system"
WG_INTERFACE="wg0"
WG_PORT=51820
WG_SUBNET="10.254.0.0/24"
CONTROLLER_PORT=8000

# ============== 颜色 ==============
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
CYAN='\033[0;36m'
NC='\033[0m'

# ============== 函数 ==============
print_banner() {
    echo -e "${CYAN}"
    cat << 'EOF'
  _     _ _         ____  ____   __        ___    _   _ 
 | |   (_) |_ ___  / ___||  _ \  \ \      / / \  | \ | |
 | |   | | __/ _ \ \___ \| | | |  \ \ /\ / / _ \ |  \| |
 | |___| | ||  __/  ___) | |_| |   \ V  V / ___ \| |\  |
 |_____|_|\__\___| |____/|____/     \_/\_/_/   \_\_| \_|
                                                        
EOF
    echo -e "${NC}"
    echo -e "${BLUE}Lite SD-WAN 一键安装程序 v1.0${NC}"
    echo ""
}

log_info() { echo -e "${BLUE}[INFO]${NC} $1"; }
log_success() { echo -e "${GREEN}[OK]${NC} $1"; }
log_warn() { echo -e "${YELLOW}[WARN]${NC} $1"; }
log_error() { echo -e "${RED}[ERROR]${NC} $1"; exit 1; }

check_root() {
    if [ "$EUID" -ne 0 ]; then
        log_error "请使用 root 权限运行: sudo $0"
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
    log_info "操作系统: $OS $OS_VERSION"
}

detect_arch() {
    ARCH=$(uname -m)
    case $ARCH in
        x86_64)  ARCH_SUFFIX="linux-amd64" ;;
        aarch64) ARCH_SUFFIX="linux-arm64" ;;
        armv7l)  ARCH_SUFFIX="linux-armv7" ;;
        *) log_error "不支持的架构: $ARCH" ;;
    esac
    log_info "系统架构: $ARCH ($ARCH_SUFFIX)"
}

install_deps() {
    log_info "安装依赖..."
    
    case $OS in
        ubuntu|debian)
            apt-get update -qq
            apt-get install -y -qq curl wget wireguard wireguard-tools
            ;;
        centos|rhel|rocky|almalinux)
            yum install -y epel-release 2>/dev/null || true
            yum install -y curl wget wireguard-tools
            ;;
        fedora)
            dnf install -y curl wget wireguard-tools
            ;;
        arch|manjaro)
            pacman -Sy --noconfirm curl wget wireguard-tools
            ;;
        *)
            log_warn "未知系统，请确保已安装 curl, wget, wireguard-tools"
            ;;
    esac
    
    log_success "依赖安装完成"
}

get_latest_version() {
    log_info "获取最新版本..."
    LATEST_VERSION=$(curl -sL "https://api.github.com/repos/${GITHUB_REPO}/releases/latest" | grep '"tag_name"' | sed -E 's/.*"([^"]+)".*/\1/')
    
    if [ -z "$LATEST_VERSION" ]; then
        log_warn "无法获取最新版本，使用 main 分支"
        LATEST_VERSION="main"
    fi
    
    log_info "版本: $LATEST_VERSION"
}

download_binaries() {
    log_info "下载二进制文件..."
    
    local base_url="https://github.com/${GITHUB_REPO}/releases/download/${LATEST_VERSION}"
    
    # 尝试下载预编译版本
    if curl -sLf "${base_url}/sdwan-controller-${ARCH_SUFFIX}" -o /tmp/sdwan-controller 2>/dev/null; then
        curl -sL "${base_url}/sdwan-agent-${ARCH_SUFFIX}" -o /tmp/sdwan-agent
        
        mv /tmp/sdwan-controller "$INSTALL_DIR/sdwan-controller"
        mv /tmp/sdwan-agent "$INSTALL_DIR/sdwan-agent"
        chmod +x "$INSTALL_DIR/sdwan-controller" "$INSTALL_DIR/sdwan-agent"
        
        log_success "二进制文件下载完成"
    else
        log_warn "无法下载预编译版本，尝试从源码编译..."
        build_from_source
    fi
}

build_from_source() {
    log_info "从源码编译..."
    
    # 安装 Go
    if ! command -v go &> /dev/null; then
        log_info "安装 Go..."
        case $OS in
            ubuntu|debian) apt-get install -y -qq golang ;;
            centos|rhel|rocky|almalinux|fedora) yum install -y golang ;;
            arch|manjaro) pacman -Sy --noconfirm go ;;
        esac
    fi
    
    # 克隆并编译
    local tmp_dir=$(mktemp -d)
    git clone --depth 1 "https://github.com/${GITHUB_REPO}.git" "$tmp_dir"
    cd "$tmp_dir"
    
    go build -o "$INSTALL_DIR/sdwan-controller" ./cmd/controller
    go build -o "$INSTALL_DIR/sdwan-agent" ./cmd/agent
    
    rm -rf "$tmp_dir"
    log_success "编译完成"
}

setup_wireguard() {
    log_info "配置 WireGuard..."
    
    mkdir -p /etc/wireguard
    
    # 生成密钥
    if [ ! -f /etc/wireguard/privatekey ]; then
        wg genkey | tee /etc/wireguard/privatekey | wg pubkey > /etc/wireguard/publickey
        chmod 600 /etc/wireguard/privatekey
    fi
    
    log_success "WireGuard 密钥已生成"
}

configure_kernel() {
    log_info "配置内核参数..."
    
    cat > /etc/sysctl.d/99-sdwan.conf << 'EOF'
net.ipv4.ip_forward = 1
net.ipv4.conf.all.forwarding = 1
net.ipv4.conf.all.rp_filter = 0
net.ipv4.conf.default.rp_filter = 0
EOF
    
    sysctl -p /etc/sysctl.d/99-sdwan.conf > /dev/null 2>&1
    log_success "内核参数配置完成"
}

interactive_setup() {
    echo ""
    echo -e "${CYAN}========================================${NC}"
    echo -e "${CYAN}       节点配置向导${NC}"
    echo -e "${CYAN}========================================${NC}"
    echo ""
    
    # 选择角色
    echo "请选择节点角色:"
    echo "  1) Controller + Agent (中心节点)"
    echo "  2) Agent (普通节点)"
    read -p "选择 [1/2]: " role_choice
    
    case $role_choice in
        1) NODE_ROLE="controller" ;;
        *) NODE_ROLE="agent" ;;
    esac
    
    # WireGuard IP
    read -p "本机 WireGuard IP (如 10.254.0.1): " NODE_WG_IP
    [ -z "$NODE_WG_IP" ] && log_error "WireGuard IP 不能为空"
    
    # 公网 IP
    DEFAULT_PUBLIC_IP=$(curl -s ifconfig.me 2>/dev/null || curl -s ip.sb 2>/dev/null || echo "")
    read -p "本机公网 IP [$DEFAULT_PUBLIC_IP]: " NODE_PUBLIC_IP
    NODE_PUBLIC_IP=${NODE_PUBLIC_IP:-$DEFAULT_PUBLIC_IP}
    
    # Controller 地址
    if [ "$NODE_ROLE" = "agent" ]; then
        read -p "Controller 地址 (如 http://10.254.0.1:8000): " CONTROLLER_URL
        [ -z "$CONTROLLER_URL" ] && log_error "Controller 地址不能为空"
    else
        CONTROLLER_URL="http://${NODE_WG_IP}:${CONTROLLER_PORT}"
    fi
    
    # 对等节点
    echo ""
    echo "添加对等节点 (格式: WG_IP,公网IP,公钥)"
    echo "输入空行结束"
    
    PEERS=()
    PEER_IPS=()
    while true; do
        read -p "对等节点: " peer_info
        [ -z "$peer_info" ] && break
        PEERS+=("$peer_info")
        IFS=',' read -r peer_wg_ip _ _ <<< "$peer_info"
        PEER_IPS+=("$peer_wg_ip")
    done
    
    # 确认
    echo ""
    echo -e "${GREEN}配置确认:${NC}"
    echo "  角色: $NODE_ROLE"
    echo "  WireGuard IP: $NODE_WG_IP"
    echo "  公网 IP: $NODE_PUBLIC_IP"
    echo "  Controller: $CONTROLLER_URL"
    echo "  对等节点: ${#PEERS[@]} 个"
    echo ""
    
    read -p "确认配置? [Y/n]: " confirm
    [[ "$confirm" =~ ^[Nn] ]] && exit 0
}

generate_configs() {
    log_info "生成配置文件..."
    
    mkdir -p "$CONFIG_DIR"
    
    # WireGuard 配置
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
    
    # Agent 配置
    cat > "$CONFIG_DIR/agent_config.yaml" << EOF
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
EOF
    
    for ip in "${PEER_IPS[@]}"; do
        echo "    - \"$ip\"" >> "$CONFIG_DIR/agent_config.yaml"
    done
    
    # Controller 配置
    if [ "$NODE_ROLE" = "controller" ]; then
        cat > "$CONFIG_DIR/controller_config.yaml" << EOF
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
    
    log_success "配置文件生成完成"
}

install_services() {
    log_info "安装 systemd 服务..."
    
    # Agent 服务
    cat > "$SYSTEMD_DIR/sdwan-agent.service" << EOF
[Unit]
Description=SD-WAN Agent
After=network.target wg-quick@$WG_INTERFACE.service
Wants=wg-quick@$WG_INTERFACE.service

[Service]
Type=simple
ExecStart=$INSTALL_DIR/sdwan-agent -config $CONFIG_DIR/agent_config.yaml
Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target
EOF
    
    # Controller 服务
    if [ "$NODE_ROLE" = "controller" ]; then
        cat > "$SYSTEMD_DIR/sdwan-controller.service" << EOF
[Unit]
Description=SD-WAN Controller
After=network.target

[Service]
Type=simple
ExecStart=$INSTALL_DIR/sdwan-controller -config $CONFIG_DIR/controller_config.yaml
Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target
EOF
    fi
    
    systemctl daemon-reload
    log_success "服务安装完成"
}

start_services() {
    log_info "启动服务..."
    
    # WireGuard
    systemctl enable wg-quick@$WG_INTERFACE 2>/dev/null || true
    systemctl start wg-quick@$WG_INTERFACE 2>/dev/null || wg-quick up $WG_INTERFACE 2>/dev/null || true
    
    # Controller
    if [ "$NODE_ROLE" = "controller" ]; then
        systemctl enable sdwan-controller
        systemctl start sdwan-controller
        sleep 2
    fi
    
    # Agent
    systemctl enable sdwan-agent
    systemctl start sdwan-agent
    
    log_success "服务启动完成"
}

show_result() {
    local public_key=$(cat /etc/wireguard/publickey)
    
    echo ""
    echo -e "${GREEN}╔════════════════════════════════════════╗${NC}"
    echo -e "${GREEN}║         安装完成！                     ║${NC}"
    echo -e "${GREEN}╚════════════════════════════════════════╝${NC}"
    echo ""
    echo -e "${CYAN}本机信息:${NC}"
    echo "  角色: $NODE_ROLE"
    echo "  WireGuard IP: $NODE_WG_IP"
    echo "  公钥: $public_key"
    echo ""
    echo -e "${CYAN}常用命令:${NC}"
    echo "  wg show                           # 查看 WireGuard 状态"
    echo "  journalctl -u sdwan-agent -f      # 查看 Agent 日志"
    if [ "$NODE_ROLE" = "controller" ]; then
        echo "  journalctl -u sdwan-controller -f # 查看 Controller 日志"
        echo "  curl localhost:$CONTROLLER_PORT/health  # 健康检查"
    fi
    echo ""
    echo -e "${YELLOW}分享给其他节点:${NC}"
    echo "  $NODE_WG_IP,$NODE_PUBLIC_IP,$public_key"
    echo ""
}

# ============== 主程序 ==============
main() {
    print_banner
    check_root
    detect_os
    detect_arch
    install_deps
    get_latest_version
    download_binaries
    setup_wireguard
    configure_kernel
    interactive_setup
    generate_configs
    install_services
    start_services
    show_result
}

main "$@"

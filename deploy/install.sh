#!/bin/bash
#
# Lite SD-WAN 一键安装脚本 v2.2
#
# 用法:
#   curl -sSL https://raw.githubusercontent.com/holygeek00/lite-sdwan/main/deploy/install.sh | sudo bash
#   curl -sSL ... | sudo bash -s -- --role controller --wg-ip 10.254.0.1  # 非交互模式
#
# 参数:
#   --role        节点角色: controller 或 agent
#   --wg-ip       WireGuard IP 地址
#   --public-ip   公网 IP 地址
#   --controller  Controller URL (agent 模式必需)
#   --peers       对等节点列表，逗号分隔
#   --skip-wg     跳过 WireGuard 配置
#   --uninstall   卸载 SD-WAN
#   --add-peer    添加对等节点
#   --show-info   显示本节点信息
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
VERSION="2.2"

# 命令行参数
ARG_ROLE=""
ARG_WG_IP=""
ARG_PUBLIC_IP=""
ARG_CONTROLLER=""
ARG_PEERS=""
ARG_SKIP_WG=false
ARG_UNINSTALL=false
ARG_ADD_PEER=false
ARG_SHOW_INFO=false
ARG_MANAGE=false

# ============== 颜色 ==============
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
CYAN='\033[0;36m'
BOLD='\033[1m'
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
    echo -e "${BLUE}Lite SD-WAN 一键安装程序 v${VERSION}${NC}"
    echo -e "${BLUE}GitHub: https://github.com/${GITHUB_REPO}${NC}"
    echo ""
}

usage() {
    echo "用法: $0 [选项]"
    echo ""
    echo "选项:"
    echo "  --role <controller|agent>  节点角色"
    echo "  --wg-ip <IP>               WireGuard IP 地址"
    echo "  --public-ip <IP>           公网 IP 地址"
    echo "  --controller <URL>         Controller URL (agent 模式)"
    echo "  --peers <peer1,peer2,...>  对等节点 (格式: wg_ip:pub_ip:pubkey)"
    echo "  --skip-wg                  跳过 WireGuard 配置"
    echo "  --uninstall                卸载 SD-WAN"
    echo "  -h, --help                 显示帮助"
    echo ""
    echo "示例:"
    echo "  # 交互式安装"
    echo "  curl -sSL .../install.sh | sudo bash"
    echo ""
    echo "  # Controller 节点"
    echo "  sudo bash install.sh --role controller --wg-ip 10.254.0.1"
    echo ""
    echo "  # Agent 节点"
    echo "  sudo bash install.sh --role agent --wg-ip 10.254.0.2 --controller http://10.254.0.1:8000"
    echo ""
    echo "  # 管理已安装的节点"
    echo "  sudo bash install.sh --add-peer    # 添加对等节点"
    echo "  sudo bash install.sh --show-info   # 显示本节点信息"
    exit 0
}

parse_args() {
    while [[ $# -gt 0 ]]; do
        case $1 in
            --role) ARG_ROLE="$2"; shift 2 ;;
            --wg-ip) ARG_WG_IP="$2"; shift 2 ;;
            --public-ip) ARG_PUBLIC_IP="$2"; shift 2 ;;
            --controller) ARG_CONTROLLER="$2"; shift 2 ;;
            --peers) ARG_PEERS="$2"; shift 2 ;;
            --skip-wg) ARG_SKIP_WG=true; shift ;;
            --uninstall) ARG_UNINSTALL=true; shift ;;
            --add-peer) ARG_ADD_PEER=true; shift ;;
            --show-info) ARG_SHOW_INFO=true; shift ;;
            -h|--help) usage ;;
            *) shift ;;
        esac
    done
}

log_info() { echo -e "${BLUE}[INFO]${NC} $1"; }
log_success() { echo -e "${GREEN}[✓]${NC} $1"; }
log_warn() { echo -e "${YELLOW}[!]${NC} $1"; }
log_error() { echo -e "${RED}[✗]${NC} $1"; exit 1; }
log_step() { echo -e "${BOLD}${CYAN}==>${NC} ${BOLD}$1${NC}"; }

spinner() {
    local pid=$1
    local delay=0.1
    local spinstr='|/-\'
    while ps -p $pid > /dev/null 2>&1; do
        local temp=${spinstr#?}
        printf " [%c]  " "$spinstr"
        local spinstr=$temp${spinstr%"$temp"}
        sleep $delay
        printf "\b\b\b\b\b\b"
    done
    printf "      \b\b\b\b\b\b"
}

check_root() {
    if [ "$EUID" -ne 0 ]; then
        log_error "请使用 root 权限运行: sudo $0"
    fi
}

# 卸载函数
uninstall() {
    log_step "卸载 Lite SD-WAN..."
    
    UNAME_S=$(uname -s)
    
    if [ "$UNAME_S" = "Darwin" ]; then
        # macOS 卸载
        local LAUNCHD_DIR="/Library/LaunchDaemons"
        
        launchctl unload "$LAUNCHD_DIR/com.sdwan.agent.plist" 2>/dev/null || true
        launchctl unload "$LAUNCHD_DIR/com.sdwan.controller.plist" 2>/dev/null || true
        
        rm -f "$LAUNCHD_DIR/com.sdwan.agent.plist"
        rm -f "$LAUNCHD_DIR/com.sdwan.controller.plist"
        
        wg-quick down $WG_INTERFACE 2>/dev/null || true
    else
        # Linux 卸载
        systemctl stop sdwan-agent 2>/dev/null || true
        systemctl stop sdwan-controller 2>/dev/null || true
        systemctl disable sdwan-agent 2>/dev/null || true
        systemctl disable sdwan-controller 2>/dev/null || true
        
        wg-quick down $WG_INTERFACE 2>/dev/null || true
        systemctl disable wg-quick@$WG_INTERFACE 2>/dev/null || true
        
        rm -f "$SYSTEMD_DIR/sdwan-agent.service" "$SYSTEMD_DIR/sdwan-controller.service"
        systemctl daemon-reload
    fi
    
    # 通用清理
    rm -f "$INSTALL_DIR/sdwan-controller" "$INSTALL_DIR/sdwan-agent"
    rm -rf "$CONFIG_DIR"
    rm -f /etc/wireguard/$WG_INTERFACE.conf
    
    log_success "卸载完成"
    exit 0
}

# 显示本节点信息
show_node_info() {
    if [ ! -f /etc/wireguard/publickey ]; then
        log_error "未找到 WireGuard 公钥，请先完成安装"
    fi
    
    local public_key=$(cat /etc/wireguard/publickey)
    
    # 解析 WireGuard IP
    local wg_ip=""
    
    # 方法1: 从 WireGuard 配置文件读取
    if [ -f /etc/wireguard/$WG_INTERFACE.conf ]; then
        wg_ip=$(grep -E "^Address\s*=" /etc/wireguard/$WG_INTERFACE.conf 2>/dev/null | head -1 | sed 's/.*=\s*//' | sed 's/\/.*$//' | tr -d ' ')
    fi
    
    # 方法2: 从 Agent 配置文件读取
    if [ -z "$wg_ip" ]; then
        if [ -f "$CONFIG_DIR/agent_config.yaml" ]; then
            wg_ip=$(grep -E "^agent_id:" "$CONFIG_DIR/agent_config.yaml" 2>/dev/null | sed 's/.*:\s*//' | tr -d '"' | tr -d ' ')
        fi
    fi
    
    # 方法3: 从网络接口获取 (Linux)
    if [ -z "$wg_ip" ] && command -v ip &> /dev/null; then
        wg_ip=$(ip -4 addr show $WG_INTERFACE 2>/dev/null | grep -oP 'inet \K[\d.]+' | head -1)
    fi
    
    # 方法4: 从网络接口获取 (macOS)
    if [ -z "$wg_ip" ] && [ "$(uname -s)" = "Darwin" ]; then
        wg_ip=$(ifconfig $WG_INTERFACE 2>/dev/null | grep 'inet ' | awk '{print $2}' | head -1)
    fi
    
    [ -z "$wg_ip" ] && wg_ip="未配置"
    
    # 获取公网 IPv4 地址 (优先 IPv4)
    local public_ip="未知"
    public_ip=$(curl -4 -s --connect-timeout 5 ifconfig.me 2>/dev/null) || \
    public_ip=$(curl -4 -s --connect-timeout 5 ip.sb 2>/dev/null) || \
    public_ip=$(curl -4 -s --connect-timeout 5 ipinfo.io/ip 2>/dev/null) || \
    public_ip=$(curl -s --connect-timeout 5 ifconfig.me 2>/dev/null) || \
    public_ip="未知"
    
    echo ""
    echo -e "${CYAN}═══════════════════════════════════════════${NC}"
    echo -e "${CYAN}  本节点信息${NC}"
    echo -e "${CYAN}═══════════════════════════════════════════${NC}"
    echo ""
    echo "  WireGuard IP: $wg_ip"
    echo "  公网 IP:      $public_ip"
    echo "  公钥:         $public_key"
    echo ""
    echo -e "${YELLOW}  复制此行分享给其他节点:${NC}"
    echo "  $wg_ip,$public_ip,$public_key"
    echo ""
    
    # 检查配置文件状态
    if [ ! -f /etc/wireguard/$WG_INTERFACE.conf ]; then
        echo -e "${RED}  警告: WireGuard 配置文件不存在！${NC}"
        echo "  请重新运行安装脚本完成配置"
        echo ""
    fi
    
    # 显示当前对等节点
    echo -e "${CYAN}当前对等节点:${NC}"
    if [ -f /etc/wireguard/$WG_INTERFACE.conf ]; then
        local peer_count=$(grep -c "^\[Peer\]" /etc/wireguard/$WG_INTERFACE.conf 2>/dev/null || echo "0")
        if [ "$peer_count" = "0" ]; then
            echo "  (无)"
        else
            grep -A3 "^\[Peer\]" /etc/wireguard/$WG_INTERFACE.conf 2>/dev/null | grep -E "(PublicKey|Endpoint|AllowedIPs)" | while read line; do
                echo "  $line"
            done
        fi
    else
        echo "  (配置文件不存在)"
    fi
    echo ""
}

# 添加对等节点
add_peer() {
    if [ ! -f /etc/wireguard/$WG_INTERFACE.conf ]; then
        log_warn "未找到 WireGuard 配置文件"
        echo ""
        echo "请先完成安装配置，然后再添加对等节点"
        echo "运行: sudo bash install.sh"
        echo ""
        exit 1
    fi
    
    echo ""
    echo -e "${CYAN}添加对等节点${NC}"
    echo "请输入对等节点信息 (格式: WG_IP,公网IP,公钥)"
    echo "例如: 10.254.0.2,1.2.3.4,abc123def456..."
    echo ""
    
    read -p "对等节点信息: " peer_info
    
    if [ -z "$peer_info" ]; then
        log_error "输入不能为空"
    fi
    
    # 解析输入
    IFS=',' read -r peer_wg_ip peer_public_ip peer_pubkey <<< "$peer_info"
    
    if [ -z "$peer_wg_ip" ] || [ -z "$peer_public_ip" ] || [ -z "$peer_pubkey" ]; then
        log_error "格式错误，请使用: WG_IP,公网IP,公钥"
    fi
    
    # 检查是否已存在
    if grep -q "$peer_pubkey" /etc/wireguard/$WG_INTERFACE.conf 2>/dev/null; then
        log_warn "该对等节点已存在"
        exit 0
    fi
    
    # 添加到配置文件
    cat >> /etc/wireguard/$WG_INTERFACE.conf << EOF

[Peer]
PublicKey = $peer_pubkey
Endpoint = $peer_public_ip:$WG_PORT
AllowedIPs = $peer_wg_ip/32
PersistentKeepalive = 25
EOF
    
    log_success "对等节点已添加到配置文件"
    
    # 添加到 agent 配置
    if [ -f "$CONFIG_DIR/agent_config.yaml" ]; then
        # 检查是否已存在
        if ! grep -q "$peer_wg_ip" "$CONFIG_DIR/agent_config.yaml" 2>/dev/null; then
            # 在 peer_ips 下添加
            sed -i.bak "/peer_ips:/a\\    - \"$peer_wg_ip\"" "$CONFIG_DIR/agent_config.yaml" 2>/dev/null || \
            sed -i '' "/peer_ips:/a\\
    - \"$peer_wg_ip\"" "$CONFIG_DIR/agent_config.yaml"
            log_success "已添加到 Agent 配置"
        fi
    fi
    
    # 重启 WireGuard
    echo ""
    read -p "是否立即重启 WireGuard 使配置生效? [Y/n]: " restart_wg
    if [[ ! "$restart_wg" =~ ^[Nn] ]]; then
        wg-quick down $WG_INTERFACE 2>/dev/null || true
        wg-quick up $WG_INTERFACE
        log_success "WireGuard 已重启"
        
        # 重启 Agent
        if [ "$(uname -s)" = "Darwin" ]; then
            launchctl stop com.sdwan.agent 2>/dev/null || true
            launchctl start com.sdwan.agent 2>/dev/null || true
        else
            systemctl restart sdwan-agent 2>/dev/null || true
        fi
        log_success "Agent 已重启"
    else
        echo ""
        echo "请手动重启 WireGuard:"
        echo "  sudo wg-quick down $WG_INTERFACE && sudo wg-quick up $WG_INTERFACE"
    fi
    
    echo ""
    log_success "对等节点添加完成!"
    exit 0
}

# 管理菜单
manage_menu() {
    # 检查配置是否完整
    local config_incomplete=false
    local missing_configs=""
    
    if [ ! -f /etc/wireguard/$WG_INTERFACE.conf ]; then
        config_incomplete=true
        missing_configs="WireGuard配置"
    fi
    
    if [ ! -f "$CONFIG_DIR/agent_config.yaml" ]; then
        config_incomplete=true
        if [ -n "$missing_configs" ]; then
            missing_configs="$missing_configs, Agent配置"
        else
            missing_configs="Agent配置"
        fi
    fi
    
    echo ""
    echo -e "${CYAN}═══════════════════════════════════════════${NC}"
    echo -e "${CYAN}  SD-WAN 管理菜单${NC}"
    echo -e "${CYAN}═══════════════════════════════════════════${NC}"
    echo ""
    
    if [ "$config_incomplete" = true ]; then
        echo -e "  ${RED}0) 完成配置 (缺失: $missing_configs)${NC}"
    fi
    
    echo "  1) 显示本节点信息"
    echo "  2) 添加对等节点"
    echo "  3) 查看 WireGuard 状态"
    echo "  4) 重启服务"
    echo "  5) 重新安装"
    echo "  6) 卸载"
    echo "  7) 退出"
    echo ""
    
    read -p "请选择 [0-7]: " choice
    
    case $choice in
        0)
            if [ "$config_incomplete" = true ]; then
                log_step "开始完成配置..."
                # 重新运行配置流程
                ARG_ROLE=""
                # 确保 stdin 可用于交互
                if [ ! -t 0 ]; then
                    if [ -e /dev/tty ]; then
                        exec < /dev/tty
                    else
                        log_error "无法获取终端输入，请直接运行脚本而不是通过管道"
                    fi
                fi
                setup_wireguard
                configure_kernel
                interactive_setup
                generate_configs
                install_services
                start_services
                show_result
            else
                log_info "配置已完整"
            fi
            ;;
        1) show_node_info ;;
        2) add_peer ;;
        3) 
            echo ""
            if ! command -v wg &> /dev/null; then
                log_error "WireGuard 未安装"
            fi
            if ! wg show $WG_INTERFACE &> /dev/null; then
                log_warn "WireGuard 接口 $WG_INTERFACE 未启动"
                echo ""
                if [ -f /etc/wireguard/$WG_INTERFACE.conf ]; then
                    echo "配置文件存在，尝试启动..."
                    wg-quick up $WG_INTERFACE && log_success "WireGuard 已启动"
                    echo ""
                    wg show $WG_INTERFACE
                else
                    echo "配置文件不存在: /etc/wireguard/$WG_INTERFACE.conf"
                    echo "请先完成配置 (选项 0 或 5)"
                fi
            else
                wg show $WG_INTERFACE
            fi
            echo ""
            ;;
        4) 
            if [ "$(uname -s)" = "Darwin" ]; then
                launchctl stop com.sdwan.agent 2>/dev/null || true
                launchctl start com.sdwan.agent 2>/dev/null || true
            else
                systemctl restart sdwan-agent 2>/dev/null || true
            fi
            log_success "服务已重启"
            ;;
        5)
            log_step "开始重新安装..."
            # 重新安装
            ARG_ROLE=""
            # 确保 stdin 可用于交互
            if [ ! -t 0 ]; then
                if [ -e /dev/tty ]; then
                    exec < /dev/tty
                else
                    log_error "无法获取终端输入，请直接运行脚本而不是通过管道"
                fi
            fi
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
            ;;
        6) uninstall ;;
        7) exit 0 ;;
        *) log_error "无效选择" ;;
    esac
}

detect_os() {
    UNAME_S=$(uname -s)
    case "$UNAME_S" in
        Linux)
            if [ -f /etc/os-release ]; then
                . /etc/os-release
                OS=$ID
                OS_VERSION=$VERSION_ID
            else
                OS="linux"
            fi
            OS_TYPE="linux"
            ;;
        Darwin)
            OS="macos"
            OS_VERSION=$(sw_vers -productVersion 2>/dev/null || echo "unknown")
            OS_TYPE="darwin"
            ;;
        *)
            OS="unknown"
            OS_TYPE="unknown"
            ;;
    esac
    log_info "操作系统: ${BOLD}$OS $OS_VERSION${NC}"
}

detect_arch() {
    ARCH=$(uname -m)
    case $ARCH in
        x86_64|amd64)
            if [ "$OS_TYPE" = "darwin" ]; then
                ARCH_SUFFIX="darwin-amd64"
            else
                ARCH_SUFFIX="linux-amd64"
            fi
            ;;
        aarch64|arm64)
            if [ "$OS_TYPE" = "darwin" ]; then
                ARCH_SUFFIX="darwin-arm64"
            else
                ARCH_SUFFIX="linux-arm64"
            fi
            ;;
        armv7l)
            ARCH_SUFFIX="linux-armv7"
            ;;
        *)
            log_error "不支持的架构: $ARCH"
            ;;
    esac
    log_info "系统架构: ${BOLD}$ARCH${NC} ($ARCH_SUFFIX)"
}

install_deps() {
    log_step "安装系统依赖..."
    
    case $OS in
        ubuntu|debian)
            log_info "使用 apt 安装依赖..."
            apt-get update -qq
            apt-get install -y curl wget wireguard wireguard-tools
            if [ $? -ne 0 ]; then
                log_error "依赖安装失败，请检查网络连接或手动安装: apt install wireguard wireguard-tools"
            fi
            ;;
        centos|rhel|rocky|almalinux)
            log_info "使用 yum 安装依赖..."
            yum install -y epel-release || true
            yum install -y curl wget wireguard-tools
            if [ $? -ne 0 ]; then
                log_error "依赖安装失败，请检查网络连接或手动安装: yum install wireguard-tools"
            fi
            ;;
        fedora)
            log_info "使用 dnf 安装依赖..."
            dnf install -y curl wget wireguard-tools
            if [ $? -ne 0 ]; then
                log_error "依赖安装失败，请检查网络连接或手动安装: dnf install wireguard-tools"
            fi
            ;;
        arch|manjaro)
            log_info "使用 pacman 安装依赖..."
            pacman -Sy --noconfirm curl wget wireguard-tools
            if [ $? -ne 0 ]; then
                log_error "依赖安装失败，请检查网络连接或手动安装: pacman -S wireguard-tools"
            fi
            ;;
        macos)
            # macOS: 使用 Homebrew
            if ! command -v brew &> /dev/null; then
                log_error "未检测到 Homebrew，请先安装: https://brew.sh
                
安装 Homebrew:
  /bin/bash -c \"\$(curl -fsSL https://raw.githubusercontent.com/Homebrew/install/HEAD/install.sh)\"

然后重新运行此脚本"
            fi
            
            log_info "使用 Homebrew 安装 wireguard-tools..."
            if ! brew list wireguard-tools &>/dev/null; then
                brew install wireguard-tools
                if [ $? -ne 0 ]; then
                    log_error "WireGuard 安装失败，请手动运行: brew install wireguard-tools"
                fi
            else
                log_info "wireguard-tools 已安装"
            fi
            ;;
        *)
            log_warn "未知系统，请确保已安装 curl, wget, wireguard-tools"
            ;;
    esac
    
    # 验证 wg 命令是否可用
    if ! command -v wg &> /dev/null; then
        log_error "WireGuard 未正确安装，请手动安装 wireguard-tools 后重试"
    fi
    
    log_success "依赖安装完成"
}

get_latest_version() {
    log_step "获取最新版本..."
    LATEST_VERSION=$(curl -sL "https://api.github.com/repos/${GITHUB_REPO}/releases/latest" 2>/dev/null | grep '"tag_name"' | sed -E 's/.*"([^"]+)".*/\1/')
    
    if [ -z "$LATEST_VERSION" ]; then
        log_warn "无法获取最新版本，使用 main 分支"
        LATEST_VERSION="main"
    fi
    
    log_info "版本: ${BOLD}$LATEST_VERSION${NC}"
}

download_binaries() {
    log_step "下载二进制文件..."
    
    local base_url="https://github.com/${GITHUB_REPO}/releases/download/${LATEST_VERSION}"
    
    # 尝试下载预编译版本
    echo -n "  下载 sdwan-controller... "
    if curl -sLf "${base_url}/sdwan-controller-${ARCH_SUFFIX}" -o /tmp/sdwan-controller 2>/dev/null; then
        echo -e "${GREEN}完成${NC}"
        
        echo -n "  下载 sdwan-agent... "
        curl -sL "${base_url}/sdwan-agent-${ARCH_SUFFIX}" -o /tmp/sdwan-agent
        echo -e "${GREEN}完成${NC}"
        
        mv /tmp/sdwan-controller "$INSTALL_DIR/sdwan-controller"
        mv /tmp/sdwan-agent "$INSTALL_DIR/sdwan-agent"
        chmod +x "$INSTALL_DIR/sdwan-controller" "$INSTALL_DIR/sdwan-agent"
        
        log_success "二进制文件下载完成"
    else
        echo -e "${YELLOW}失败${NC}"
        log_warn "无法下载预编译版本，尝试从源码编译..."
        build_from_source
    fi
}

build_from_source() {
    log_step "从源码编译..."
    
    # 安装 Go
    if ! command -v go &> /dev/null; then
        log_info "安装 Go..."
        case $OS in
            ubuntu|debian) apt-get install -y -qq golang >/dev/null 2>&1 ;;
            centos|rhel|rocky|almalinux|fedora) yum install -y golang >/dev/null 2>&1 ;;
            arch|manjaro) pacman -Sy --noconfirm go >/dev/null 2>&1 ;;
            macos) brew install go >/dev/null 2>&1 ;;
        esac
    fi
    
    # 安装 git
    if ! command -v git &> /dev/null; then
        log_info "安装 git..."
        case $OS in
            ubuntu|debian) apt-get install -y -qq git >/dev/null 2>&1 ;;
            centos|rhel|rocky|almalinux|fedora) yum install -y git >/dev/null 2>&1 ;;
            arch|manjaro) pacman -Sy --noconfirm git >/dev/null 2>&1 ;;
            macos) brew install git >/dev/null 2>&1 ;;
        esac
    fi
    
    # 克隆并编译
    local tmp_dir=$(mktemp -d)
    log_info "克隆仓库..."
    git clone --depth 1 "https://github.com/${GITHUB_REPO}.git" "$tmp_dir" >/dev/null 2>&1
    cd "$tmp_dir"
    
    log_info "编译 controller..."
    go build -o "$INSTALL_DIR/sdwan-controller" ./cmd/controller
    
    log_info "编译 agent..."
    go build -o "$INSTALL_DIR/sdwan-agent" ./cmd/agent
    
    cd /
    rm -rf "$tmp_dir"
    log_success "编译完成"
}

setup_wireguard() {
    if [ "$ARG_SKIP_WG" = true ]; then
        log_info "跳过 WireGuard 配置"
        return
    fi
    
    log_step "配置 WireGuard..."
    
    mkdir -p /etc/wireguard
    
    # 生成密钥
    if [ ! -f /etc/wireguard/privatekey ]; then
        wg genkey | tee /etc/wireguard/privatekey | wg pubkey > /etc/wireguard/publickey
        chmod 600 /etc/wireguard/privatekey
        log_success "WireGuard 密钥已生成"
    else
        log_info "使用已有的 WireGuard 密钥"
    fi
}

configure_kernel() {
    log_step "配置内核参数..."
    
    if [ "$OS_TYPE" = "darwin" ]; then
        # macOS: 启用 IP 转发
        sysctl -w net.inet.ip.forwarding=1 > /dev/null 2>&1 || true
        log_info "macOS IP 转发已启用 (重启后需重新设置)"
    else
        # Linux
        cat > /etc/sysctl.d/99-sdwan.conf << 'EOF'
net.ipv4.ip_forward = 1
net.ipv4.conf.all.forwarding = 1
net.ipv4.conf.all.rp_filter = 0
net.ipv4.conf.default.rp_filter = 0
EOF
        sysctl -p /etc/sysctl.d/99-sdwan.conf > /dev/null 2>&1
    fi
    
    log_success "内核参数配置完成"
}

interactive_setup() {
    # 非交互模式：使用命令行参数
    if [ -n "$ARG_ROLE" ]; then
        log_info "使用非交互模式..."
        
        NODE_ROLE="$ARG_ROLE"
        NODE_WG_IP="$ARG_WG_IP"
        NODE_PUBLIC_IP="$ARG_PUBLIC_IP"
        
        # 验证必需参数
        [ -z "$NODE_WG_IP" ] && log_error "非交互模式需要 --wg-ip 参数"
        
        # 自动获取公网 IP (优先 IPv4)
        if [ -z "$NODE_PUBLIC_IP" ]; then
            NODE_PUBLIC_IP=$(curl -4 -s --connect-timeout 5 ifconfig.me 2>/dev/null || curl -4 -s --connect-timeout 5 ip.sb 2>/dev/null || curl -4 -s --connect-timeout 5 ipinfo.io/ip 2>/dev/null || echo "")
        fi
        
        # Controller URL
        if [ "$NODE_ROLE" = "agent" ]; then
            [ -z "$ARG_CONTROLLER" ] && log_error "Agent 模式需要 --controller 参数"
            CONTROLLER_URL="$ARG_CONTROLLER"
        else
            CONTROLLER_URL="http://${NODE_WG_IP}:${CONTROLLER_PORT}"
        fi
        
        # 解析对等节点
        PEERS=()
        PEER_IPS=()
        if [ -n "$ARG_PEERS" ]; then
            IFS=',' read -ra peer_list <<< "$ARG_PEERS"
            for peer in "${peer_list[@]}"; do
                PEERS+=("$peer")
                IFS=':' read -r peer_wg_ip _ _ <<< "$peer"
                PEER_IPS+=("$peer_wg_ip")
            done
        fi
        
        log_info "角色: $NODE_ROLE"
        log_info "WireGuard IP: $NODE_WG_IP"
        log_info "公网 IP: $NODE_PUBLIC_IP"
        log_info "Controller: $CONTROLLER_URL"
        return
    fi
    
    # 检查是否为管道模式（stdin 不是终端）
    # 如果 stdin 不是终端，尝试重定向到 /dev/tty
    if [ ! -t 0 ]; then
        if [ -e /dev/tty ]; then
            exec < /dev/tty
        else
            log_error "交互模式需要终端输入。请使用以下方式之一：
        
  1) 下载后执行:
     curl -sSL <URL>/install.sh -o /tmp/install.sh
     sudo bash /tmp/install.sh
     
  2) 非交互模式:
     curl -sSL <URL>/install.sh | sudo bash -s -- --role controller --wg-ip 10.254.0.1
     curl -sSL <URL>/install.sh | sudo bash -s -- --role agent --wg-ip 10.254.0.2 --controller http://10.254.0.1:8000"
        fi
    fi
    
    # 交互模式
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
    
    # 公网 IP (优先 IPv4)
    DEFAULT_PUBLIC_IP=$(curl -4 -s --connect-timeout 5 ifconfig.me 2>/dev/null || curl -4 -s --connect-timeout 5 ip.sb 2>/dev/null || curl -4 -s --connect-timeout 5 ipinfo.io/ip 2>/dev/null || echo "")
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
    log_step "生成配置文件..."
    
    # 检查必要变量
    if [ -z "$NODE_WG_IP" ]; then
        log_error "NODE_WG_IP 未设置，无法生成配置"
    fi
    
    log_info "NODE_WG_IP: $NODE_WG_IP"
    log_info "NODE_ROLE: $NODE_ROLE"
    log_info "CONTROLLER_URL: $CONTROLLER_URL"
    
    mkdir -p "$CONFIG_DIR"
    mkdir -p /etc/wireguard
    
    # 检查私钥是否存在
    if [ ! -f /etc/wireguard/privatekey ]; then
        log_error "WireGuard 私钥不存在，请先运行 setup_wireguard"
    fi
    
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
    log_success "WireGuard 配置已创建: /etc/wireguard/$WG_INTERFACE.conf"
    
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
    
    # 验证配置文件已生成
    if [ ! -f /etc/wireguard/$WG_INTERFACE.conf ]; then
        log_error "WireGuard 配置文件生成失败"
    fi
    
    if [ ! -f "$CONFIG_DIR/agent_config.yaml" ]; then
        log_error "Agent 配置文件生成失败"
    fi
    
    log_success "配置文件生成完成"
    log_info "WireGuard 配置: /etc/wireguard/$WG_INTERFACE.conf"
    log_info "Agent 配置: $CONFIG_DIR/agent_config.yaml"
}

install_services() {
    log_info "安装服务..."
    
    if [ "$OS_TYPE" = "darwin" ]; then
        install_launchd_services
    else
        install_systemd_services
    fi
}

install_launchd_services() {
    local LAUNCHD_DIR="/Library/LaunchDaemons"
    
    # Agent plist
    cat > "$LAUNCHD_DIR/com.sdwan.agent.plist" << EOF
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>com.sdwan.agent</string>
    <key>ProgramArguments</key>
    <array>
        <string>$INSTALL_DIR/sdwan-agent</string>
        <string>-config</string>
        <string>$CONFIG_DIR/agent_config.yaml</string>
    </array>
    <key>RunAtLoad</key>
    <true/>
    <key>KeepAlive</key>
    <true/>
    <key>StandardOutPath</key>
    <string>/var/log/sdwan-agent.log</string>
    <key>StandardErrorPath</key>
    <string>/var/log/sdwan-agent.log</string>
</dict>
</plist>
EOF
    
    # Controller plist
    if [ "$NODE_ROLE" = "controller" ]; then
        cat > "$LAUNCHD_DIR/com.sdwan.controller.plist" << EOF
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>com.sdwan.controller</string>
    <key>ProgramArguments</key>
    <array>
        <string>$INSTALL_DIR/sdwan-controller</string>
        <string>-config</string>
        <string>$CONFIG_DIR/controller_config.yaml</string>
    </array>
    <key>RunAtLoad</key>
    <true/>
    <key>KeepAlive</key>
    <true/>
    <key>StandardOutPath</key>
    <string>/var/log/sdwan-controller.log</string>
    <key>StandardErrorPath</key>
    <string>/var/log/sdwan-controller.log</string>
</dict>
</plist>
EOF
    fi
    
    log_success "launchd 服务安装完成"
}

install_systemd_services() {
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
    log_success "systemd 服务安装完成"
}

start_services() {
    log_info "启动服务..."
    
    if [ "$OS_TYPE" = "darwin" ]; then
        start_launchd_services
    else
        start_systemd_services
    fi
}

start_launchd_services() {
    local LAUNCHD_DIR="/Library/LaunchDaemons"
    
    # WireGuard (macOS 使用 wg-quick 手动启动)
    wg-quick up $WG_INTERFACE 2>/dev/null || true
    
    # Controller
    if [ "$NODE_ROLE" = "controller" ]; then
        launchctl load "$LAUNCHD_DIR/com.sdwan.controller.plist" 2>/dev/null || true
        sleep 2
    fi
    
    # Agent
    launchctl load "$LAUNCHD_DIR/com.sdwan.agent.plist" 2>/dev/null || true
    
    log_success "服务启动完成"
}

start_systemd_services() {
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
    echo -e "${CYAN}═══════════════════════════════════════════${NC}"
    echo -e "${CYAN}  本节点信息 (分享给其他节点)${NC}"
    echo -e "${CYAN}═══════════════════════════════════════════${NC}"
    echo ""
    echo "  WireGuard IP: $NODE_WG_IP"
    echo "  公网 IP:      $NODE_PUBLIC_IP"
    echo "  公钥:         $public_key"
    echo ""
    echo -e "${YELLOW}  复制此行分享给其他节点:${NC}"
    echo "  $NODE_WG_IP,$NODE_PUBLIC_IP,$public_key"
    echo ""
    echo -e "${CYAN}═══════════════════════════════════════════${NC}"
    echo ""
    
    # 如果没有配置对等节点，显示如何添加
    if [ ${#PEERS[@]} -eq 0 ]; then
        echo -e "${YELLOW}添加对等节点:${NC}"
        echo "  编辑 /etc/wireguard/$WG_INTERFACE.conf，添加:"
        echo ""
        echo "  [Peer]"
        echo "  PublicKey = <对方公钥>"
        echo "  Endpoint = <对方公网IP>:$WG_PORT"
        echo "  AllowedIPs = <对方WG_IP>/32"
        echo "  PersistentKeepalive = 25"
        echo ""
        echo "  或运行: sudo bash install.sh --add-peer"
        echo ""
    fi
    
    echo -e "${CYAN}常用命令:${NC}"
    echo "  wg show                           # 查看 WireGuard 状态"
    
    if [ "$OS_TYPE" = "darwin" ]; then
        echo "  tail -f /var/log/sdwan-agent.log  # 查看 Agent 日志"
        if [ "$NODE_ROLE" = "controller" ]; then
            echo "  tail -f /var/log/sdwan-controller.log # 查看 Controller 日志"
            echo "  curl localhost:$CONTROLLER_PORT/health  # 健康检查"
        fi
        echo ""
        echo -e "${CYAN}服务管理 (macOS):${NC}"
        echo "  sudo launchctl list | grep sdwan  # 查看服务状态"
        echo "  sudo launchctl stop com.sdwan.agent  # 停止 Agent"
        echo "  sudo launchctl start com.sdwan.agent # 启动 Agent"
    else
        echo "  journalctl -u sdwan-agent -f      # 查看 Agent 日志"
        if [ "$NODE_ROLE" = "controller" ]; then
            echo "  journalctl -u sdwan-controller -f # 查看 Controller 日志"
            echo "  curl localhost:$CONTROLLER_PORT/health  # 健康检查"
        fi
    fi
    
    echo ""
    echo -e "${CYAN}管理命令:${NC}"
    echo "  再次运行此脚本可进入管理菜单"
    echo "  sudo bash install.sh --add-peer   # 快速添加对等节点"
    echo "  sudo bash install.sh --show-info  # 显示本节点信息"
    echo ""
}

# ============== 主程序 ==============
main() {
    # 解析命令行参数
    parse_args "$@"
    
    # 处理卸载
    if [ "$ARG_UNINSTALL" = true ]; then
        check_root
        uninstall
    fi
    
    # 处理显示节点信息
    if [ "$ARG_SHOW_INFO" = true ]; then
        check_root
        show_node_info
        exit 0
    fi
    
    # 处理添加对等节点
    if [ "$ARG_ADD_PEER" = true ]; then
        check_root
        # 如果是管道模式，重定向 stdin
        if [ ! -t 0 ] && [ -e /dev/tty ]; then
            exec < /dev/tty
        fi
        add_peer
    fi
    
    print_banner
    check_root
    detect_os
    detect_arch
    
    # 如果是管道模式且需要交互，重定向 stdin 到 /dev/tty
    if [ -z "$ARG_ROLE" ] && [ ! -t 0 ] && [ -e /dev/tty ]; then
        exec < /dev/tty
    fi
    
    # 检查是否已安装，显示管理菜单
    if [ -z "$ARG_ROLE" ] && [ -f "$INSTALL_DIR/sdwan-agent" ]; then
        log_info "检测到已安装的 SD-WAN"
        manage_menu
        exit 0
    fi
    
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

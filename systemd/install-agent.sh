#!/bin/bash
#
# SD-WAN Agent 安装脚本 (Go 版本)
#
# 用法: sudo ./install-agent.sh
#

set -e

INSTALL_DIR="/usr/local/bin"
CONFIG_DIR="/etc/sdwan"
SYSTEMD_DIR="/etc/systemd/system"
GITHUB_REPO="holygeek00/lite-sdwan"

RED='\033[0;31m'
GREEN='\033[0;32m'
BLUE='\033[0;34m'
NC='\033[0m'

log_info() { echo -e "${BLUE}[INFO]${NC} $1"; }
log_success() { echo -e "${GREEN}[✓]${NC} $1"; }
log_error() { echo -e "${RED}[✗]${NC} $1"; exit 1; }

echo "=== SD-WAN Agent 安装 (Go 版本) ==="
echo ""

# 检查 root 权限
if [ "$EUID" -ne 0 ]; then
    log_error "请使用 root 权限运行: sudo $0"
fi

# 检查 WireGuard
if ! command -v wg &> /dev/null; then
    log_error "WireGuard 未安装，请先安装 wireguard-tools"
fi

# 检测架构
ARCH=$(uname -m)
case $ARCH in
    x86_64)  ARCH_SUFFIX="linux-amd64" ;;
    aarch64) ARCH_SUFFIX="linux-arm64" ;;
    armv7l)  ARCH_SUFFIX="linux-armv7" ;;
    *) log_error "不支持的架构: $ARCH" ;;
esac

log_info "架构: $ARCH ($ARCH_SUFFIX)"

# 创建目录
log_info "创建目录..."
mkdir -p "$CONFIG_DIR"

# 下载二进制文件
log_info "下载 sdwan-agent..."
if curl -sLf "https://github.com/${GITHUB_REPO}/releases/latest/download/sdwan-agent-${ARCH_SUFFIX}" -o "$INSTALL_DIR/sdwan-agent"; then
    chmod +x "$INSTALL_DIR/sdwan-agent"
    log_success "下载完成"
else
    log_error "下载失败，请检查网络或手动编译"
fi

# 复制配置模板
if [ ! -f "$CONFIG_DIR/agent_config.yaml" ]; then
    log_info "创建配置模板..."
    cat > "$CONFIG_DIR/agent_config.yaml" << 'EOF'
agent_id: "10.254.0.X"  # 修改为本机 WireGuard IP

controller:
  url: "http://10.254.0.1:8000"  # 修改为 Controller 地址
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
    - "10.254.0.1"  # 添加对等节点 IP
EOF
    log_success "配置模板已创建: $CONFIG_DIR/agent_config.yaml"
else
    log_info "配置文件已存在"
fi

# 安装 systemd 服务
log_info "安装 systemd 服务..."
cat > "$SYSTEMD_DIR/sdwan-agent.service" << EOF
[Unit]
Description=SD-WAN Agent
After=network.target wg-quick@wg0.service
Wants=wg-quick@wg0.service

[Service]
Type=simple
ExecStart=$INSTALL_DIR/sdwan-agent -config $CONFIG_DIR/agent_config.yaml
Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target
EOF

systemctl daemon-reload
systemctl enable sdwan-agent.service

echo ""
echo -e "${GREEN}=== 安装完成 ===${NC}"
echo ""
echo "后续步骤:"
echo "  1. 编辑配置: nano $CONFIG_DIR/agent_config.yaml"
echo "  2. 配置 WireGuard: wg-quick up wg0"
echo "  3. 启动服务: systemctl start sdwan-agent"
echo "  4. 查看状态: systemctl status sdwan-agent"
echo "  5. 查看日志: journalctl -u sdwan-agent -f"
echo ""

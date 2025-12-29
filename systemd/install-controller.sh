#!/bin/bash
#
# SD-WAN Controller 安装脚本 (Go 版本)
#
# 用法: sudo ./install-controller.sh
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

echo "=== SD-WAN Controller 安装 (Go 版本) ==="
echo ""

# 检查 root 权限
if [ "$EUID" -ne 0 ]; then
    log_error "请使用 root 权限运行: sudo $0"
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
log_info "下载 sdwan-controller..."
if curl -sLf "https://github.com/${GITHUB_REPO}/releases/latest/download/sdwan-controller-${ARCH_SUFFIX}" -o "$INSTALL_DIR/sdwan-controller"; then
    chmod +x "$INSTALL_DIR/sdwan-controller"
    log_success "下载完成"
else
    log_error "下载失败，请检查网络或手动编译"
fi

# 复制配置模板
if [ ! -f "$CONFIG_DIR/controller_config.yaml" ]; then
    log_info "创建配置模板..."
    cat > "$CONFIG_DIR/controller_config.yaml" << 'EOF'
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
    log_success "配置模板已创建: $CONFIG_DIR/controller_config.yaml"
else
    log_info "配置文件已存在"
fi

# 安装 systemd 服务
log_info "安装 systemd 服务..."
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

systemctl daemon-reload
systemctl enable sdwan-controller.service

# 配置防火墙
if command -v ufw &> /dev/null; then
    log_info "配置防火墙..."
    ufw allow 8000/tcp comment "SD-WAN Controller API" 2>/dev/null || true
fi

echo ""
echo -e "${GREEN}=== 安装完成 ===${NC}"
echo ""
echo "后续步骤:"
echo "  1. 查看配置: cat $CONFIG_DIR/controller_config.yaml"
echo "  2. 启动服务: systemctl start sdwan-controller"
echo "  3. 查看状态: systemctl status sdwan-controller"
echo "  4. 查看日志: journalctl -u sdwan-controller -f"
echo "  5. 测试 API: curl http://localhost:8000/health"
echo ""

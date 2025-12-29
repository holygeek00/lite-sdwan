#!/bin/bash
#
# Lite SD-WAN 快速安装脚本
#
# 一行命令安装:
#   curl -sSL https://raw.githubusercontent.com/holygeek00/lite-sdwan/main/deploy/install.sh | sudo bash
#
# 或者:
#   wget -qO- https://raw.githubusercontent.com/holygeek00/lite-sdwan/main/deploy/install.sh | sudo bash
#

set -e

REPO_URL="https://github.com/holygeek00/lite-sdwan"
INSTALL_DIR="/opt/sdwan"
VERSION="main"

RED='\033[0;31m'
GREEN='\033[0;32m'
BLUE='\033[0;34m'
NC='\033[0m'

echo -e "${BLUE}"
cat << 'BANNER'
  _     _ _         ____  ____   __        ___    _   _ 
 | |   (_) |_ ___  / ___||  _ \  \ \      / / \  | \ | |
 | |   | | __/ _ \ \___ \| | | |  \ \ /\ / / _ \ |  \| |
 | |___| | ||  __/  ___) | |_| |   \ V  V / ___ \| |\  |
 |_____|_|\__\___| |____/|____/     \_/\_/_/   \_\_| \_|
                                                        
BANNER
echo -e "${NC}"
echo "Lite SD-WAN 快速安装程序"
echo ""

# 检查 root 权限
if [ "$EUID" -ne 0 ]; then
    echo -e "${RED}错误: 请使用 root 权限运行${NC}"
    echo "用法: sudo $0"
    exit 1
fi

# 检测操作系统
if [ -f /etc/os-release ]; then
    . /etc/os-release
    OS=$ID
else
    OS="unknown"
fi

echo "检测到操作系统: $OS"

# 安装 git
echo "安装 git..."
case $OS in
    ubuntu|debian)
        apt-get update -qq
        apt-get install -y -qq git
        ;;
    centos|rhel|rocky|almalinux|fedora)
        dnf install -y git 2>/dev/null || yum install -y git
        ;;
    arch|manjaro)
        pacman -Sy --noconfirm git
        ;;
esac

# 克隆仓库
echo "下载 SD-WAN..."
if [ -d "$INSTALL_DIR" ]; then
    echo "更新现有安装..."
    cd "$INSTALL_DIR"
    git pull origin $VERSION
else
    git clone --depth 1 -b $VERSION "$REPO_URL" "$INSTALL_DIR"
    cd "$INSTALL_DIR"
fi

# 运行部署脚本
echo ""
echo "启动部署向导..."
echo ""
chmod +x deploy/deploy.sh
exec deploy/deploy.sh

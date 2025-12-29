#!/bin/bash
#
# Lite SD-WAN 快速安装脚本
#
# 一行命令安装:
#   curl -sSL https://raw.githubusercontent.com/holygeek00/lite-sdwan/main/deploy/quick_install.sh | sudo bash
#
# 这个脚本会下载并执行完整的 install.sh 安装脚本
#

set -e

INSTALL_URL="https://raw.githubusercontent.com/holygeek00/lite-sdwan/main/deploy/install.sh"

# 下载并执行安装脚本
curl -sSL "$INSTALL_URL" | bash -s -- "$@"

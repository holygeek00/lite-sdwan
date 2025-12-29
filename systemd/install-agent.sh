#!/bin/bash
# Installation script for SD-WAN Agent
# Usage: sudo ./install-agent.sh

set -e

echo "=== SD-WAN Agent Installation ==="

# Check if running as root
if [ "$EUID" -ne 0 ]; then
    echo "Error: This script must be run as root"
    exit 1
fi

# Check if WireGuard is installed
if ! command -v wg &> /dev/null; then
    echo "Error: WireGuard is not installed. Please install it first."
    exit 1
fi

# Check if Python 3 is installed
if ! command -v python3 &> /dev/null; then
    echo "Error: Python 3 is not installed. Please install it first."
    exit 1
fi

# Create directories
echo "Creating directories..."
mkdir -p /opt/sdwan
mkdir -p /etc/sdwan
mkdir -p /var/log/sdwan

# Copy application files
echo "Copying application files..."
cp -r agent controller config models.py /opt/sdwan/

# Install Python dependencies
echo "Installing Python dependencies..."
pip3 install -r requirements.txt

# Copy configuration template
if [ ! -f /etc/sdwan/agent_config.yaml ]; then
    echo "Copying configuration template..."
    cp config/agent_config.yaml /etc/sdwan/agent_config.yaml
    echo "WARNING: Please edit /etc/sdwan/agent_config.yaml before starting the service"
else
    echo "Configuration file already exists at /etc/sdwan/agent_config.yaml"
fi

# Copy systemd service file
echo "Installing systemd service..."
cp systemd/sdwan-agent.service /etc/systemd/system/

# Reload systemd
echo "Reloading systemd..."
systemctl daemon-reload

# Enable service
echo "Enabling service..."
systemctl enable sdwan-agent.service

echo ""
echo "=== Installation Complete ==="
echo ""
echo "Next steps:"
echo "1. Edit the configuration file: nano /etc/sdwan/agent_config.yaml"
echo "2. Configure WireGuard: wg-quick up wg0"
echo "3. Start the service: systemctl start sdwan-agent.service"
echo "4. Check status: systemctl status sdwan-agent.service"
echo "5. View logs: journalctl -u sdwan-agent.service -f"
echo ""

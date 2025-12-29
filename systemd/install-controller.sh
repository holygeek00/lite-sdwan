#!/bin/bash
# Installation script for SD-WAN Controller
# Usage: sudo ./install-controller.sh

set -e

echo "=== SD-WAN Controller Installation ==="

# Check if running as root
if [ "$EUID" -ne 0 ]; then
    echo "Error: This script must be run as root"
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

# Create dedicated user
if ! id -u sdwan &> /dev/null; then
    echo "Creating sdwan user..."
    useradd -r -s /bin/false sdwan
fi

# Set ownership
echo "Setting file ownership..."
chown -R sdwan:sdwan /opt/sdwan
chown -R sdwan:sdwan /var/log/sdwan

# Copy configuration template
if [ ! -f /etc/sdwan/controller_config.yaml ]; then
    echo "Copying configuration template..."
    cp config/controller_config.yaml /etc/sdwan/controller_config.yaml
    chown sdwan:sdwan /etc/sdwan/controller_config.yaml
    echo "WARNING: Please edit /etc/sdwan/controller_config.yaml if needed"
else
    echo "Configuration file already exists at /etc/sdwan/controller_config.yaml"
fi

# Copy systemd service file
echo "Installing systemd service..."
cp systemd/sdwan-controller.service /etc/systemd/system/

# Reload systemd
echo "Reloading systemd..."
systemctl daemon-reload

# Enable service
echo "Enabling service..."
systemctl enable sdwan-controller.service

# Configure firewall (if ufw is installed)
if command -v ufw &> /dev/null; then
    echo "Configuring firewall..."
    ufw allow 8000/tcp comment "SD-WAN Controller API"
fi

echo ""
echo "=== Installation Complete ==="
echo ""
echo "Next steps:"
echo "1. Review configuration: nano /etc/sdwan/controller_config.yaml"
echo "2. Start the service: systemctl start sdwan-controller.service"
echo "3. Check status: systemctl status sdwan-controller.service"
echo "4. View logs: journalctl -u sdwan-controller.service -f"
echo "5. Test API: curl http://localhost:8000/health"
echo ""

# Systemd Service Files for Lite SD-WAN Routing System

This directory contains systemd service files for running the SD-WAN Agent and Controller as system daemons.

## Files

- `sdwan-agent.service` - Service file for the Agent (runs on all nodes)
- `sdwan-controller.service` - Service file for the Controller (runs on central node)

## Installation

### Prerequisites

1. Install Python dependencies:
```bash
pip3 install -r requirements.txt
```

2. Install the application to `/opt/sdwan`:
```bash
sudo mkdir -p /opt/sdwan
sudo cp -r agent controller config models.py /opt/sdwan/
```

3. Create configuration directory:
```bash
sudo mkdir -p /etc/sdwan
```

### Agent Installation

1. Copy and customize the agent configuration:
```bash
sudo cp config/agent_config.yaml /etc/sdwan/agent_config.yaml
sudo nano /etc/sdwan/agent_config.yaml
```

Edit the configuration to set:
- `agent_id`: Unique identifier for this node (e.g., "10.254.0.2")
- `controller.url`: URL of the Controller (e.g., "http://10.254.0.1:8000")
- `network.peer_ips`: List of all peer node IPs

2. Copy the service file:
```bash
sudo cp systemd/sdwan-agent.service /etc/systemd/system/
```

3. Reload systemd and enable the service:
```bash
sudo systemctl daemon-reload
sudo systemctl enable sdwan-agent.service
```

4. Start the service:
```bash
sudo systemctl start sdwan-agent.service
```

5. Check status:
```bash
sudo systemctl status sdwan-agent.service
sudo journalctl -u sdwan-agent.service -f
```

### Controller Installation

1. Create a dedicated user for the controller:
```bash
sudo useradd -r -s /bin/false sdwan
sudo chown -R sdwan:sdwan /opt/sdwan
```

2. Copy and customize the controller configuration:
```bash
sudo cp config/controller_config.yaml /etc/sdwan/controller_config.yaml
sudo nano /etc/sdwan/controller_config.yaml
```

Edit the configuration to set:
- `server.listen_address`: Listen address (usually "0.0.0.0")
- `server.port`: Listen port (default 8000)
- `algorithm.penalty_factor`: Cost penalty for packet loss (default 100)
- `algorithm.hysteresis`: Route switching threshold (default 0.15)

3. Copy the service file:
```bash
sudo cp systemd/sdwan-controller.service /etc/systemd/system/
```

4. Reload systemd and enable the service:
```bash
sudo systemctl daemon-reload
sudo systemctl enable sdwan-controller.service
```

5. Start the service:
```bash
sudo systemctl start sdwan-controller.service
```

6. Check status:
```bash
sudo systemctl status sdwan-controller.service
sudo journalctl -u sdwan-controller.service -f
```

## Service Management

### Start services
```bash
sudo systemctl start sdwan-agent.service
sudo systemctl start sdwan-controller.service
```

### Stop services
```bash
sudo systemctl stop sdwan-agent.service
sudo systemctl stop sdwan-controller.service
```

### Restart services
```bash
sudo systemctl restart sdwan-agent.service
sudo systemctl restart sdwan-controller.service
```

### View logs
```bash
# View agent logs
sudo journalctl -u sdwan-agent.service -f

# View controller logs
sudo journalctl -u sdwan-controller.service -f

# View logs since last boot
sudo journalctl -u sdwan-agent.service -b
```

### Enable/disable auto-start
```bash
# Enable auto-start on boot
sudo systemctl enable sdwan-agent.service
sudo systemctl enable sdwan-controller.service

# Disable auto-start
sudo systemctl disable sdwan-agent.service
sudo systemctl disable sdwan-controller.service
```

## Troubleshooting

### Agent not starting

1. Check if WireGuard is running:
```bash
sudo wg show
```

2. Check if the configuration file is valid:
```bash
python3 -c "from config.parser import AgentConfig; AgentConfig.from_file('/etc/sdwan/agent_config.yaml')"
```

3. Check permissions:
```bash
ls -la /etc/sdwan/agent_config.yaml
```

### Controller not starting

1. Check if port 8000 is available:
```bash
sudo netstat -tlnp | grep 8000
```

2. Check if the configuration file is valid:
```bash
python3 -c "from config.parser import ControllerConfig; ControllerConfig.from_file('/etc/sdwan/controller_config.yaml')"
```

3. Check user permissions:
```bash
sudo -u sdwan python3 -m uvicorn controller.api:app --host 0.0.0.0 --port 8000
```

### Service crashes immediately

Check the journal for error messages:
```bash
sudo journalctl -u sdwan-agent.service -n 50
sudo journalctl -u sdwan-controller.service -n 50
```

## Security Notes

- The Agent service runs as `root` because it needs to modify kernel routing tables
- The Controller service runs as a dedicated `sdwan` user for security
- Both services have security hardening enabled (NoNewPrivileges, PrivateTmp, ProtectSystem)
- Log files are written to systemd journal (use `journalctl` to view)

## Requirements

- Python 3.9+
- systemd (most modern Linux distributions)
- WireGuard (for Agent nodes)
- Root privileges for Agent (to modify routing tables)

## References

- Requirements: 11.4
- Design Document: See `.kiro/specs/lite-sdwan-routing/design.md`
- Configuration: See `config/agent_config.yaml` and `config/controller_config.yaml`

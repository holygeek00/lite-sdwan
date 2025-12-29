# Lite SD-WAN Routing System - Deployment Guide

## Overview

This guide provides instructions for deploying the Lite SD-WAN Routing System in production environments using systemd.

## Prerequisites

### All Nodes
- Linux system with systemd
- Python 3.9 or higher
- Root access for installation

### Agent Nodes
- WireGuard installed and configured
- Full mesh WireGuard network established
- IP forwarding enabled in kernel

### Controller Node
- Network connectivity to all agent nodes
- Port 8000 available (or configure different port)

## Quick Start

### 1. Install Dependencies

On all nodes:
```bash
pip3 install -r requirements.txt
```

### 2. Deploy Controller (Central Node)

```bash
# Run installation script
sudo ./systemd/install-controller.sh

# Edit configuration if needed
sudo nano /etc/sdwan/controller_config.yaml

# Start the service
sudo systemctl start sdwan-controller.service

# Verify it's running
sudo systemctl status sdwan-controller.service
curl http://localhost:8000/health
```

### 3. Deploy Agents (All Nodes)

```bash
# Run installation script
sudo ./systemd/install-agent.sh

# Edit configuration (REQUIRED)
sudo nano /etc/sdwan/agent_config.yaml
# Set: agent_id, controller.url, network.peer_ips

# Ensure WireGuard is running
sudo wg-quick up wg0

# Start the service
sudo systemctl start sdwan-agent.service

# Verify it's running
sudo systemctl status sdwan-agent.service
```

## Configuration Files

### Agent Configuration (`/etc/sdwan/agent_config.yaml`)

Required fields:
- `agent_id`: Unique identifier (e.g., "10.254.0.2")
- `controller.url`: Controller API URL (e.g., "http://10.254.0.1:8000")
- `probe.interval`: Probe cycle interval in seconds (default: 5)
- `sync.interval`: Route sync interval in seconds (default: 10)
- `network.wg_interface`: WireGuard interface name (default: "wg0")
- `network.peer_ips`: List of all peer node IPs

Example:
```yaml
agent_id: "10.254.0.2"
controller:
  url: "http://10.254.0.1:8000"
  timeout: 5
probe:
  interval: 5
  timeout: 2
  window_size: 10
sync:
  interval: 10
  retry_attempts: 3
  retry_backoff: [1, 2, 4]
network:
  wg_interface: "wg0"
  subnet: "10.254.0.0/24"
  peer_ips:
    - "10.254.0.1"
    - "10.254.0.3"
```

### Controller Configuration (`/etc/sdwan/controller_config.yaml`)

Required fields:
- `server.listen_address`: Listen address (default: "0.0.0.0")
- `server.port`: Listen port (default: 8000)
- `algorithm.penalty_factor`: Packet loss penalty (default: 100)
- `algorithm.hysteresis`: Route switching threshold (default: 0.15)

Example:
```yaml
server:
  listen_address: "0.0.0.0"
  port: 8000
algorithm:
  penalty_factor: 100
  hysteresis: 0.15
topology:
  stale_threshold: 60
logging:
  level: "INFO"
  file: "/var/log/sdwan-controller.log"
```

## Service Management

### Start/Stop Services

```bash
# Start
sudo systemctl start sdwan-agent.service
sudo systemctl start sdwan-controller.service

# Stop
sudo systemctl stop sdwan-agent.service
sudo systemctl stop sdwan-controller.service

# Restart
sudo systemctl restart sdwan-agent.service
sudo systemctl restart sdwan-controller.service
```

### View Logs

```bash
# Agent logs
sudo journalctl -u sdwan-agent.service -f

# Controller logs
sudo journalctl -u sdwan-controller.service -f

# Last 100 lines
sudo journalctl -u sdwan-agent.service -n 100
```

### Enable/Disable Auto-start

```bash
# Enable (start on boot)
sudo systemctl enable sdwan-agent.service
sudo systemctl enable sdwan-controller.service

# Disable
sudo systemctl disable sdwan-agent.service
sudo systemctl disable sdwan-controller.service
```

## Verification

### 1. Check WireGuard Status

```bash
sudo wg show
```

Expected output should show all peers with recent handshakes.

### 2. Check Agent Status

```bash
sudo systemctl status sdwan-agent.service
```

Should show "active (running)" status.

### 3. Check Controller Status

```bash
sudo systemctl status sdwan-controller.service
curl http://<controller-ip>:8000/health
```

Should return JSON with status "healthy".

### 4. Verify Route Updates

On an agent node:
```bash
# Check current routes
ip route show table main | grep wg0

# Watch logs for route updates
sudo journalctl -u sdwan-agent.service -f
```

You should see routes being updated based on network conditions.

## Troubleshooting

### Agent Won't Start

1. **Check WireGuard**: Ensure WireGuard is running
   ```bash
   sudo wg show
   sudo wg-quick up wg0
   ```

2. **Check Configuration**: Validate config file
   ```bash
   python3 -c "from config.parser import AgentConfig; AgentConfig.from_file('/etc/sdwan/agent_config.yaml')"
   ```

3. **Check Logs**: Look for error messages
   ```bash
   sudo journalctl -u sdwan-agent.service -n 50
   ```

### Controller Won't Start

1. **Check Port**: Ensure port 8000 is available
   ```bash
   sudo netstat -tlnp | grep 8000
   ```

2. **Check Configuration**: Validate config file
   ```bash
   python3 -c "from config.parser import ControllerConfig; ControllerConfig.from_file('/etc/sdwan/controller_config.yaml')"
   ```

3. **Check Permissions**: Ensure sdwan user has access
   ```bash
   sudo -u sdwan ls -la /opt/sdwan
   ```

### Agent in Fallback Mode

If agent logs show "Entering fallback mode":

1. **Check Controller Connectivity**:
   ```bash
   curl http://<controller-ip>:8000/health
   ```

2. **Check Network**: Ensure agent can reach controller
   ```bash
   ping <controller-ip>
   ```

3. **Check Controller Logs**: Look for errors
   ```bash
   sudo journalctl -u sdwan-controller.service -n 50
   ```

### Routes Not Updating

1. **Check Telemetry**: Verify agent is sending data
   ```bash
   # On controller, check logs
   sudo journalctl -u sdwan-controller.service | grep "Received telemetry"
   ```

2. **Check Route Computation**: Verify controller is computing routes
   ```bash
   # On controller, check logs
   sudo journalctl -u sdwan-controller.service | grep "Computed.*routes"
   ```

3. **Check Route Application**: Verify agent is applying routes
   ```bash
   # On agent, check logs
   sudo journalctl -u sdwan-agent.service | grep "route"
   ```

## Security Considerations

### Agent Security
- Runs as root (required for route table modifications)
- Security hardening enabled in systemd service
- Only modifies routes in configured subnet (10.254.0.0/24)

### Controller Security
- Runs as dedicated `sdwan` user (non-root)
- Security hardening enabled in systemd service
- No authentication (should be deployed in trusted network)
- Consider adding firewall rules to restrict access

### Network Security
- All data plane traffic encrypted by WireGuard
- Control plane (HTTP) is unencrypted
- Deploy controller in trusted network or add TLS

## Performance Tuning

### Probe Interval
- Default: 5 seconds
- Lower values: More responsive but higher CPU/network usage
- Higher values: Less responsive but lower overhead

### Sync Interval
- Default: 10 seconds
- Should be >= probe interval
- Lower values: Faster route updates but more API calls

### Hysteresis Threshold
- Default: 0.15 (15% improvement required)
- Lower values: More aggressive route switching
- Higher values: More stable but less optimal routes

### Penalty Factor
- Default: 100 (1% loss = 100ms penalty)
- Higher values: More aggressive loss avoidance
- Lower values: More latency-focused routing

## Monitoring

### Key Metrics to Monitor

1. **Agent Health**:
   - Service status: `systemctl status sdwan-agent.service`
   - Probe success rate: Check logs for timeouts
   - Route update frequency: Check logs for route changes

2. **Controller Health**:
   - Service status: `systemctl status sdwan-controller.service`
   - API response time: Monitor `/health` endpoint
   - Active agent count: Check `/health` response

3. **Network Quality**:
   - RTT values: Check agent logs
   - Packet loss rates: Check agent logs
   - Route stability: Monitor route change frequency

### Log Rotation

Systemd journal handles log rotation automatically. To configure:

```bash
# Edit journald config
sudo nano /etc/systemd/journald.conf

# Set limits
SystemMaxUse=1G
SystemMaxFileSize=100M
```

## Backup and Recovery

### Configuration Backup

```bash
# Backup configurations
sudo tar -czf sdwan-config-backup.tar.gz /etc/sdwan/

# Restore configurations
sudo tar -xzf sdwan-config-backup.tar.gz -C /
```

### Disaster Recovery

If controller fails:
1. Agents automatically enter fallback mode
2. Routes are flushed, WireGuard default routing takes over
3. Network remains functional (direct paths only)
4. Restore controller from backup
5. Agents automatically reconnect and resume optimal routing

## References

- Requirements: `.kiro/specs/lite-sdwan-routing/requirements.md`
- Design: `.kiro/specs/lite-sdwan-routing/design.md`
- Tasks: `.kiro/specs/lite-sdwan-routing/tasks.md`
- Systemd Details: `systemd/README.md`

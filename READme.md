# Server Setup and Configuration Script

This script automates the setup of a server environment with Nomad, Consul, Docker, and various other services. It also configures UFW firewall rules for secure communication between servers.

## Prerequisites

- Ubuntu/Debian-based system
- Root or sudo privileges
- Internet connectivity
- Infisical token for secrets management

## Features

The script installs and configures:
- Caddy web server
- Redis server
- Docker and Docker Compose
- Node.js (via NVM) and Yarn
- PM2 process manager
- Git
- Infisical CLI
- Nomad
- Consul
- UFW firewall
- CNI plugins
- Various development tools

## Usage

### Basic Command Structure

```bash
sudo ./setup.sh --servers=["ip1","ip2","ip3"] --infisical_token=your_token_here --git_user=your_git_username --git_password=your_git_password
```

### Parameters

1. `--servers`: Array of server IP addresses in JSON format
   - Required
   - Format: `["ip1","ip2","ip3"]`
   - Example: `["192.168.1.1","10.0.0.1","172.16.0.1"]`

2. `--infisical_token`: Your Infisical authentication token
   - Required
   - Format: String
   - Example: `--infisical_token=your_token_here`

3. `--git_user`: Your Git username
   - Required
   - Format: String
   - Example: `--git_user=your_git_username`

### Example

```bash
sudo ./setup.sh --servers=["192.168.1.100","192.168.1.101","192.168.1.102"] --infisical_token=inf.12345.abcdef --git_user=your_git_username --git_password=your_git_password
```

## Firewall Configuration

The script automatically configures UFW with the following rules:
- Allows SSH (22)
- Allows HTTP (80)
- Allows HTTPS (443)
- Allows Nomad ports (4646, 4647, 4648)
- Allows node exporter (9100)
- Allows custom port 8889
- Allows custom port 53133
- Allows communication between specified server IPs

## Services Configuration

### Nomad
- Configured as a client
- Connects to specified server IPs
- Integrates with Consul
- Enables Docker plugin with privileged mode

### Consul
- Configured as a server with UI
- Generates encryption key
- Sets up service discovery
- Configures ACLs

### Docker
- Installed with compose plugin
- Configured for Nomad integration
- Enables privileged containers and volumes

## Post-Installation

After running the script:
1. Verify services are running:
   ```bash
   systemctl status nomad
   systemctl status consul
   systemctl status redis-server
   systemctl status caddy
   ```

2. Check UFW status:
   ```bash
   sudo ufw status
   ```

3. Verify Nomad cluster:
   ```bash
   nomad node status
   ```

## Environment Variables

The script sets up the following environment variables:
- `INFISICAL_TOKEN`: Added to `.bashrc`
- Various path and configuration variables for services

## Logs

- Installation logs are written to `/var/log/nomad-setup.log`
- Service logs can be viewed using `journalctl`:
  ```bash
  journalctl -u nomad
  journalctl -u consul
  ```

## Troubleshooting

1. If the script fails, check:
   - System requirements
   - Internet connectivity
   - Valid Infisical token
   - Valid IP addresses format

2. Common issues:
   - UFW rules not applying: Ensure UFW is installed and enabled
   - Services not starting: Check system logs with `journalctl`
   - Permission issues: Ensure script is run with sudo
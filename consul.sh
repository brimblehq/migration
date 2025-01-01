#!/bin/bash

# Consul Installation Script
# This script automates the installation and configuration of HashiCorp Consul
# Usage: ./consul.sh [server_ip]
# If server_ip is not provided, the script will automatically detect the server's IP address

set -e

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

log() {
    echo -e "${GREEN}[$(date +'%Y-%m-%dT%H:%M:%S%z')]: $1${NC}"
}

error() {
    echo -e "${RED}[ERROR]: $1${NC}" >&2
    exit 1
}

warning() {
    echo -e "${YELLOW}[WARNING]: $1${NC}"
}

if [[ $EUID -ne 0 ]]; then
   error "This script must be run as root"
fi

detect_ip() {
    local ip=$(ip route get 1 | awk '{print $(NF-2);exit}')
    
    if [ -z "$ip" ]; then
        for interface in eth0 ens3 ens4 ens5 ens6 ens7 ens8 ens9 enp0s3 enp0s8; do
            if ip addr show $interface >/dev/null 2>&1; then
                ip=$(ip addr show $interface | grep "inet\b" | awk '{print $2}' | cut -d/ -f1)
                if [ ! -z "$ip" ]; then
                    break
                fi
            fi
        done
    fi
    
    if [ -z "$ip" ]; then
        ip=$(hostname -I | awk '{print $1}')
    fi
    
    if [ -z "$ip" ]; then
        error "Could not detect server IP address. Please provide it as an argument."
    fi
    
    echo "$ip"
}

validate_ip() {
    local ip=$1
    if [[ ! $ip =~ ^[0-9]{1,3}\.[0-9]{1,3}\.[0-9]{1,3}\.[0-9]{1,3}$ ]]; then
        error "Invalid IP address format: $ip"
    fi
    
    IFS='.' read -r -a octets <<< "$ip"
    for octet in "${octets[@]}"; do
        if [[ $octet -lt 0 || $octet -gt 255 ]]; then
            error "Invalid IP address: octets must be between 0 and 255"
        fi
    done
}

if [ -z "$1" ]; then
    log "No IP address provided, attempting to detect server IP..."
    SERVER_IP=$(detect_ip)
    log "Detected IP address: $SERVER_IP"
else
    SERVER_IP=$1
    validate_ip "$SERVER_IP"
fi


check_distribution() {
    if [ -f /etc/os-release ]; then
        . /etc/os-release
        if [[ "$ID" != "ubuntu" && "$ID" != "debian" ]]; then
            warning "This script is tested on Ubuntu/Debian. Your distribution: $PRETTY_NAME"
            read -p "Do you want to continue? (y/N) " -n 1 -r
            echo
            if [[ ! $REPLY =~ ^[Yy]$ ]]; then
                error "Installation aborted"
            fi
        else
            log "Detected distribution: $PRETTY_NAME"
        fi
    else
        warning "Could not determine Linux distribution"
        read -p "Do you want to continue? (y/N) " -n 1 -r
        echo
        if [[ ! $REPLY =~ ^[Yy]$ ]]; then
            error "Installation aborted"
        fi
    fi
}


install_dependencies() {
    log "Installing required packages..."
    apt-get update -y || error "Failed to update package list"
    apt-get install -y unzip gnupg2 curl wget iproute2 || error "Failed to install dependencies"
}

install_consul() {
    log "Downloading and installing Consul..."
    CONSUL_VERSION="1.8.4"
    wget "https://releases.hashicorp.com/consul/${CONSUL_VERSION}/consul_${CONSUL_VERSION}_linux_amd64.zip" || error "Failed to download Consul"
    unzip "consul_${CONSUL_VERSION}_linux_amd64.zip" || error "Failed to unzip Consul"
    mv consul /usr/local/bin/ || error "Failed to move Consul binary"
    rm "consul_${CONSUL_VERSION}_linux_amd64.zip"
    
    INSTALLED_VERSION=$(consul --version | head -n1)
    log "Consul installed successfully: ${INSTALLED_VERSION}"
}

setup_consul_environment() {
    log "Setting up Consul environment..."
    
    groupadd --system consul 2>/dev/null || warning "Consul group already exists"
    useradd -s /sbin/nologin --system -g consul consul 2>/dev/null || warning "Consul user already exists"
    
    mkdir -p /var/lib/consul /etc/consul.d
    
    chown -R consul:consul /var/lib/consul
    chmod -R 775 /var/lib/consul
    chown -R consul:consul /etc/consul.d
}


create_systemd_service() {
    log "Creating Consul systemd service..."
    cat > /etc/systemd/system/consul.service << EOF
[Unit]
Description=Consul Service Discovery Agent
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=consul
Group=consul
ExecStart=/usr/local/bin/consul agent -server -ui \\
            -advertise=${SERVER_IP} \\
            -bind=${SERVER_IP} \\
            -data-dir=/var/lib/consul \\
            -node=consul-01 \\
            -config-dir=/etc/consul.d
ExecReload=/bin/kill -HUP \$MAINPID
KillSignal=SIGINT
TimeoutStopSec=5
Restart=on-failure
SyslogIdentifier=consul

[Install]
WantedBy=multi-user.target
EOF

    systemctl daemon-reload
}

configure_consul() {
    log "Configuring Consul..."
    
    CONSUL_KEY=$(consul keygen)
    
    cat > /etc/consul.d/config.json << EOF
{
    "bootstrap": true,
    "server": true,
    "log_level": "DEBUG",
    "enable_syslog": true,
    "datacenter": "server1",
    "addresses": {
        "http": "0.0.0.0"
    },
    "bind_addr": "${SERVER_IP}",
    "node_name": "$(hostname)",
    "data_dir": "/var/lib/consul",
    "acl_datacenter": "server1",
    "acl_default_policy": "allow",
    "encrypt": "${CONSUL_KEY}"
}
EOF
}

start_consul() {
    log "Starting Consul service..."
    systemctl start consul
    systemctl enable consul
    
    if systemctl is-active --quiet consul; then
        log "Consul service is running"
    else
        error "Failed to start Consul service"
    fi
}

main() {
    check_distribution
    log "Starting Consul installation..."
    install_dependencies
    install_consul
    setup_consul_environment
    create_systemd_service
    configure_consul
    start_consul
    log "Consul installation completed successfully!"
    log "You can access the Consul UI at http://${SERVER_IP}:8500/ui/"
}

main

exit 0
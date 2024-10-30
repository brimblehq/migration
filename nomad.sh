#!/bin/bash

parse_params() {
    local servers_param=""
    
    while [ $# -gt 0 ]; do
        case "$1" in
            --servers=*)
                servers_param="${1#*=}"
                shift
                ;;
            *)
                echo "Error: Unknown parameter '$1'"
                echo "Usage: $0 --servers=[\"ip1\",\"ip2\",\"ip3\"]"
                exit 1
                ;;
        esac
    done

    if [ -z "$servers_param" ]; then
        echo "Error: --servers parameter is required"
        echo "Usage: $0 --servers=[\"ip1\",\"ip2\",\"ip3\"]"
        exit 1
    fi

    servers_param=$(echo "$servers_param" | tr -d ' ')
    
    if [[ ! "$servers_param" =~ ^\[\".*\"\]$ ]]; then
        echo "Error: Invalid servers format. Must be in the format: [\"ip1\",\"ip2\",\"ip3\"]"
        exit 1
    fi

    echo "$servers_param"
}

SERVERS_JSON=$(parse_params "$@")

sudo apt-get update && \
  sudo apt-get install wget gpg coreutils


wget -O- https://apt.releases.hashicorp.com/gpg | \
  sudo gpg --dearmor -o /usr/share/keyrings/hashicorp-archive-keyring.gpg

echo "deb [signed-by=/usr/share/keyrings/hashicorp-archive-keyring.gpg] https://apt.releases.hashicorp.com $(lsb_release -cs) main" \
| sudo tee /etc/apt/sources.list.d/hashicorp.list


sudo apt-get update -y && sudo apt-get install nomad

export ARCH_CNI=$( [ $(uname -m) = aarch64 ] && echo arm64 || echo amd64)

export CNI_PLUGIN_VERSION=v1.5.1

curl -L -o cni-plugins.tgz "https://github.com/containernetworking/plugins/releases/download/${CNI_PLUGIN_VERSION}/cni-plugins-linux-${ARCH_CNI}-${CNI_PLUGIN_VERSION}".tgz && \

sudo apt-get install -y consul-cni

apt-get update -y

apt-get install unzip gnupg2 curl wget -y

wget https://releases.hashicorp.com/consul/1.8.4/consul_1.8.4_linux_amd64.zip

unzip consul_1.8.4_linux_amd64.zip

mv consul /usr/local/bin/

consul --version

groupadd --system consul

useradd -s /sbin/nologin --system -g consul consul

mkdir -p /var/lib/consul

mkdir /etc/consul.d

chown -R consul:consul /var/lib/consul

chmod -R 775 /var/lib/consul

chown -R consul:consul /etc/consul.d

SERVER_IP=$(hostname -I | awk '{print $1}')

SERVICE_CONTENT="[Unit]
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
WantedBy=multi-user.target"

echo "$SERVICE_CONTENT" | sudo tee /etc/systemd/system/consul.service > /dev/null

echo "Consul service created and started successfully."

CONSUL_ENCRYPTION_KEY=$(consul keygen)

CONFIG_CONTENT="{
    \"bootstrap\": true,
    \"server\": true,
    \"log_level\": \"DEBUG\",
    \"enable_syslog\": true,
    \"datacenter\": \"server1\",
    \"addresses\": {
        \"http\": \"0.0.0.0\"
    },
    \"bind_addr\": \"$SERVER_IP\",
    \"node_name\": \"ubuntu2004\",
    \"data_dir\": \"/var/lib/consul\",
    \"acl_datacenter\": \"server1\",
    \"acl_default_policy\": \"allow\",
    \"encrypt\": \"$CONSUL_ENCRYPTION_KEY\"
}"

echo "$CONFIG_CONTENT" | sudo tee /etc/consul.d/config.json > /dev/null

echo "Consul configuration created at /etc/consul.d/config.json with encryption key and server IP."

sudo systemctl daemon-reload

sudo systemctl enable consul

sudo systemctl start consul

NOMAD_CONFIG="data_dir = \"/opt/nomad\"
bind_addr = \"0.0.0.0\"

client {
  enabled = true
  servers = $SERVERS_JSON
}

advertise {
  http = \"${SERVER_IP}:4646\"
  rpc  = \"${SERVER_IP}:4647\"
  serf = \"${SERVER_IP}:4648\"
}

consul {
  address = \"127.0.0.1:8500\"
  server_service_name = \"nomad\"
  client_service_name = \"nomad-client\"
  auto_advertise = true
  server_auto_join = true
  client_auto_join = true
}

plugin \"docker\" {
  config {
    allow_privileged = true
    volumes {
      enabled = true
    }
  }
}

telemetry {
  collection_interval = \"1s\"
  disable_hostname = true
  prometheus_metrics = true
  publish_allocation_metrics = true
  publish_node_metrics = true
}"

sudo mkdir -p /etc/nomad.d

echo "$NOMAD_CONFIG" | sudo tee /etc/nomad.d/nomad.hcl > /dev/null

sudo chown -R nomad:nomad /etc/nomad.d
sudo chmod 640 /etc/nomad.d/nomad.hcl

echo "Nomad configuration has been written to /etc/nomad.d/nomad.hcl"

sudo systemctl enable nomad

sudo systemctl start nomad
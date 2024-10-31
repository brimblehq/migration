#!/bin/bash

INFISICAL_TOKEN=""

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

add_servers_to_ufw() {
    local servers_json="$1"
    
    # Remove the outer brackets and split by commas
    local server_list=${servers_json#[}
    server_list=${server_list%]}
    
    # Convert string to array using IFS
    IFS=',' read -ra server_array <<< "$server_list"
    
    echo "Adding server IPs to UFW..."
    
    for server in "${server_array[@]}"; do
        # Remove quotes from IP
        server=$(echo "$server" | tr -d '"')
        
        if [[ $server =~ ^[0-9]+\.[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
            echo "Adding rule for IP: $server"
            if sudo ufw allow from "$server"; then
                echo "✓ Successfully added UFW rule for $server"
            else
                echo "✗ Failed to add UFW rule for $server"
                exit 1
            fi
        else
            echo "Warning: Invalid IP address format: $server"
            exit 1
        fi
    done
    
    echo "All server IPs have been added to UFW."
}

if [ "$EUID" -ne 0 ]; then 
    echo "Error: Please run this script as root or with sudo"
    exit 1
fi

SERVERS_JSON=$(parse_params "$@")

is_background() {
    [[ -z $(ps -o stat= -p $$) ]] || [[ ${$(ps -o stat= -p $$)%+*} =~ "s" ]]
}

daemonize() {
    local log_file="/var/log/nomad-setup.log"
    
    if ! is_background; then
        echo "Detaching process to run in background. Logs will be written to $log_file"
        nohup "$0" "$@" >> "$log_file" 2>&1 &
        exit 0
    fi
}


for arg in "$@"; do
  case $arg in
    --infisical_token=*)
      INFISICAL_TOKEN="${arg#*=}"
      shift
      ;;
    *)
      echo "Unknown option $arg"
      exit 1
      ;;
  esac
done

if [ -z "$INFISICAL_TOKEN" ]; then
  echo "Error: --infisical_token=<value> is required"
  exit 1
fi

ENV_VAR="export INFISICAL_TOKEN=$INFISICAL_TOKEN"

sudo apt update -y
sudo apt upgrade -y

sudo apt install curl unzip wget ufw -y

sudo apt install -y debian-keyring debian-archive-keyring apt-transport-https curl
curl -1sLf 'https://dl.cloudsmith.io/public/caddy/stable/gpg.key' | sudo gpg --dearmor -o /usr/share/keyrings/caddy-stable-archive-keyring.gpg
curl -1sLf 'https://dl.cloudsmith.io/public/caddy/stable/debian.deb.txt' | sudo tee /etc/apt/sources.list.d/caddy-stable.list
sudo apt update -y
sudo apt install caddy -y

sudo systemctl enable caddy -y
sudo systemctl start caddy -y

sudo apt install redis-server -y

sudo apt-get update -y
sudo apt-get install ca-certificates curl -y
sudo install -m 0755 -d /etc/apt/keyrings
sudo curl -fsSL https://download.docker.com/linux/ubuntu/gpg -o /etc/apt/keyrings/docker.asc
sudo chmod a+r /etc/apt/keyrings/docker.asc

echo \
  "deb [arch=$(dpkg --print-architecture) signed-by=/etc/apt/keyrings/docker.asc] https://download.docker.com/linux/ubuntu \
  $(. /etc/os-release && echo "$VERSION_CODENAME") stable" | \
  sudo tee /etc/apt/sources.list.d/docker.list > /dev/null
sudo apt-get update -y

sudo apt-get install docker-ce docker-ce-cli containerd.io docker-buildx-plugin docker-compose-plugin -y

sudo apt install docker-compose -y

curl -o- https://raw.githubusercontent.com/nvm-sh/nvm/v0.40.1/install.sh | bash
source ~/.bashrc
nvm install --lts

npm install --global yarn

npm install -g pm2

curl -1sLf \
'https://dl.cloudsmith.io/public/infisical/infisical-cli/setup.deb.sh' \
| sudo -E bash

sudo apt-get update && sudo apt-get install -y infisical

sudo apt install git -y

git config --global user.email "dave@brimble.app"
git config --global user.name "Muritala David"

mkdir /brimble/runner

git clone https://github.com/brimblehq/runner /brimble/runner

cd /brimble/runner
export INFISICAL_TOKEN=value-here
yarn install && yarn build && yarn pm2

sudo systemctl enable redis-server
sudo systemctl start redis-server

curl -sSL https://nixpacks.com/install.sh | bash

systemctl daemon-reload

ENV_VAR="export INFISICAL_TOKEN=$INFISICAL_TOKEN"

if ! grep -q "$ENV_VAR" ~/.bashrc; then
    echo "$ENV_VAR" >> ~/.bashrc
    echo "INFISICAL_TOKEN has been added to .bashrc"
else
    echo "INFISICAL_TOKEN is already set in .bashrc"
fi

add_servers_to_ufw "$SERVERS_JSON"

echo "Reloading UFW..."
if sudo ufw reload; then
    echo "✓ UFW rules have been successfully updated"
else
    echo "✗ Failed to reload UFW"
    exit 1
fi

ufw allow OpenSSH
ufw allow 80/tcp
ufw allow 443
ufw allow 4646/tcp
ufw allow 4647/tcp
ufw allow 4648/tcp
ufw allow 9100/tcp
ufw allow 8889/tcp
ufw allow 53133

ufw allow OpenSSH
ufw allow 80/tcp
ufw allow 443
ufw allow 4646/tcp
ufw allow 4647/tcp
ufw allow 4648/tcp
ufw allow 9100/tcp
ufw allow 8889/tcp
ufw allow 53133

sudo ufw enable

source ~/.bashrc

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
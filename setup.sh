#!/bin/bash

INFISICAL_TOKEN=""
SERVERS_PARAM=""
GIT_USER=""
GIT_PASSWORD=""

while [ $# -gt 0 ]; do
    case "$1" in
        --servers=*)
            SERVERS_PARAM="${1#*=}"
            shift
            ;;
        --infisical_token=*)
            INFISICAL_TOKEN="${1#*=}"
            shift
            ;;
        --git_user=*)
            GIT_USER="${1#*=}"
            shift
            ;;
        --git_password=*)
            GIT_PASSWORD="${1#*=}"
            shift
            ;;
        *)
            echo "Error: Unknown parameter '$1'"
            echo -e "Usage:\n./setup.sh --servers=[\"ip1\",\"ip2\",\"ip3\"] --infisical_token=your_token --git_user=username --git_password=password"
            exit 1
            ;;
    esac
done

if [ -z "$SERVERS_PARAM" ]; then
    echo "Error: --servers parameter is required"
    echo -e "Usage:\n./setup.sh --servers=[\"ip1\",\"ip2\",\"ip3\"] --infisical_token=your_token"
    exit 1
fi

if [ -z "$INFISICAL_TOKEN" ]; then
    echo "Error: --infisical_token parameter is required"
    echo -e "Usage:\n./setup.sh --servers=[\"ip1\",\"ip2\",\"ip3\"] --infisical_token=your_token"
    exit 1
fi

if [ -z "$GIT_USER" ] || [ -z "$GIT_PASSWORD" ]; then
    echo "Error: Git credentials are required (--git_user and --git_password)"
    echo -e "Usage:\n./setup.sh --servers=[\"ip1\",\"ip2\",\"ip3\"] --infisical_token=your_token --git_user=username --git_password=password"
    exit 1
fi

SERVERS_PARAM=$(echo "$SERVERS_PARAM" | tr -d ' ')

if [[ ! "$SERVERS_PARAM" =~ ^\[.*\]$ ]]; then
    echo "Error: Invalid servers format. Must be an array enclosed in square brackets."
    echo -e "Usage:\n./setup.sh --servers=[\"ip1\",\"ip2\",\"ip3\"] --infisical_token=your_token\n"
    echo "Example:"
    echo -e "./setup.sh --servers=[\"192.168.1.100\",\"192.168.1.101\",\"192.168.1.102\"] --infisical_token=your_token"
    exit 1
fi

if [ "$EUID" -ne 0 ]; then 
    echo "Error: Please run this script as root or with sudo"
    exit 1
fi

echo "Valid servers configuration detected: $SERVERS_PARAM"
echo "Infisical token provided: $INFISICAL_TOKEN"


add_servers_to_ufw() {
    local servers_json="$1"
    
    local server_list=${servers_json#[}
    server_list=${server_list%]}
    
    IFS=',' read -ra server_array <<< "$server_list"
    
    echo "Adding server IPs to UFW..."
    
    for server in "${server_array[@]}"; do
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

sudo systemctl enable caddy
sudo systemctl start caddy

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

export NVM_DIR="$HOME/.nvm"
[ -s "$NVM_DIR/nvm.sh" ] && \. "$NVM_DIR/nvm.sh"
[ -s "$NVM_DIR/bash_completion" ] && \. "$NVM_DIR/bash_completion"

NVM_INIT='export NVM_DIR="$HOME/.nvm"
[ -s "$NVM_DIR/nvm.sh" ] && \. "$NVM_DIR/nvm.sh"
[ -s "$NVM_DIR/bash_completion" ] && \. "$NVM_DIR/bash_completion"'

if ! grep -q "NVM_DIR" ~/.bashrc; then
    echo "$NVM_INIT" >> ~/.bashrc
    echo "Added NVM initialization to .bashrc"
fi

source ~/.bashrc

command -v nvm

nvm install --lts
nvm use --lts

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

ENCODED_PASSWORD=$(printf %s "$GIT_PASSWORD" | jq -sRr @uri)

git clone https://"$GIT_USER":"$ENCODED_PASSWORD"@github.com/brimblehq/runner /brimble/runner

if [ $? -ne 0 ]; then
    echo "Error: Failed to clone repository. Please check your Git credentials."
    exit 1
fi

cd /brimble/runner
export INFISICAL_TOKEN=$INFISICAL_TOKEN
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

add_servers_to_ufw() {
    local servers_json="$1"
    
    local server_list=${servers_json#[}
    server_list=${server_list%]}
    
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
  sudo apt-get install wget gpg coreutils -y

wget -O- https://apt.releases.hashicorp.com/gpg | \
  sudo gpg --dearmor -o /usr/share/keyrings/hashicorp-archive-keyring.gpg

echo "deb [signed-by=/usr/share/keyrings/hashicorp-archive-keyring.gpg] https://apt.releases.hashicorp.com $(lsb_release -cs) main" \
| sudo tee /etc/apt/sources.list.d/hashicorp.list

sudo apt-get update -y && sudo apt-get install nomad

export ARCH_CNI=$( [ $(uname -m) = aarch64 ] && echo arm64 || echo amd64)

export CNI_PLUGIN_VERSION=v1.5.1

curl -L -o cni-plugins.tgz "https://github.com/containernetworking/plugins/releases/download/${CNI_PLUGIN_VERSION}/cni-plugins-linux-${ARCH_CNI}-${CNI_PLUGIN_VERSION}".tgz

curl -1sLf 'https://cdn.brimble.io/consul.sh' | sudo bash

format_servers_for_nomad() {
    local servers_json="$1"
    servers_json=${servers_json#[}
    servers_json=${servers_json%]}
    IFS=',' read -ra server_array <<< "$servers_json"
    
    local nomad_servers="["
    for server in "${server_array[@]}"; do
        server=$(echo "$server" | tr -d '"' | tr -d ' ')
        nomad_servers="$nomad_servers\"$server\", "
    done
    nomad_servers=${nomad_servers%, }]
    
    echo "$nomad_servers"
}

NOMAD_SERVERS=$(format_servers_for_nomad "$SERVERS_PARAM")

NOMAD_CONFIG="data_dir = \"/opt/nomad\"
bind_addr = \"0.0.0.0\"

client {
  enabled = true
  servers = $NOMAD_SERVERS
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

echo "Runner setup completed successfully 🎉"

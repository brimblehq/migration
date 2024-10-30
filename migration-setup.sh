#!/bin/bash

INFISICAL_TOKEN=""

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

sudo apt install curl unzip wget -y

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

ufw allow from 157.90.225.125
ufw allow from 172.104.238.198
ufw allow from 178.79.136.41
ufw allow from 172.232.133.28
ufw allow from 172.232.159.15
ufw allow from 139.162.232.67
ufw allow from 45.79.42.23
ufw allow from 192.155.82.17
ufw allow from 95.217.240.111

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
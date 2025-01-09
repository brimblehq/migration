terraform {
  required_providers {
    digitalocean = {
      source  = "digitalocean/digitalocean"
      version = "~> 2.0"
    }
  }
}

provider "digitalocean" {
  token = var.digitalocean_token
}

# Generate SSH Key Pair
resource "tls_private_key" "ssh_key" {
  algorithm = "RSA"
  rsa_bits  = 4096
}

resource "local_file" "private_key" {
  content  = tls_private_key.ssh_key.private_key_pem
  filename = "./brimble_key.pem"
}

# resource "local_file" "public_key" {
#   content  = tls_private_key.ssh_key.public_key_openssh
#   filename = "${path.module}/brimble_key.pub"
# }

# Upload the Public Key to DigitalOcean
resource "digitalocean_ssh_key" "brimble_ssh_key" {
  name       = "brimble-ssh-key"
  public_key = tls_private_key.ssh_key.public_key_openssh
}

# Create a VPC Network
resource "digitalocean_vpc" "brimble_vpc" {
  name   = "brimble-vpc"
  region = var.region
}

# Create a Droplet with the SSH Key
resource "digitalocean_droplet" "brimble_droplet" {
  for_each   = { for i in range(var.instance_count) : i => format("brimble-droplet-%02d", i) }
  name   = each.value
  region = var.region
  size   = "s-2vcpu-8gb"
  image  = "ubuntu-20-04-x64"
  vpc_uuid = digitalocean_vpc.brimble_vpc.id

  # Add SSH key for secure access
  ssh_keys = [digitalocean_ssh_key.brimble_ssh_key.fingerprint]
}
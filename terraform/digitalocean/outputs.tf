# Output Droplet Public IP Address
output "droplet_ip" {
  description = "The public IP address of the DigitalOcean droplet"
  value       = digitalocean_droplet.brimble_droplet.ipv4_address
}

# Output Path to Local Private Key File
output "private_key_path" {
  description = "The local file path of the generated private SSH key"
  value       = local_file.private_key.filename
}

# Output Path to Local Public Key File
output "public_key_path" {
  description = "The local file path of the generated public SSH key"
  value       = local_file.public_key.filename
}
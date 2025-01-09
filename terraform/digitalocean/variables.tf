variable "digitalocean_token" {
  description = "DigitalOcean API token"
  type        = string
  sensitive   = true
}

variable "instance_count" {
  description = "The Number of instances to be launched"
  type        = number
  default     = 1
}

variable "region" {
  description = "Region to deploy droplets"
  type        = string
  default     = "nyc1"
}
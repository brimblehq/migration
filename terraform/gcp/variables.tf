variable "project_id" {
  description = "GCP project ID"
  type        = string
  default = "Nomad-setup"
}

variable "region" {
  description = "GCP region"
  type        = string
  default     = "us-central1"
}

variable "zone" {
  description = "GCP zone"
  type        = string
  default     = "us-central1-a"
}

variable "machine_type" {
  description = "Machine type for GCP instances"
  type        = string
  default     = "e2-standard-2"
}

variable "image" {
  description = "Source image for the instance template"
  type        = string
  default     = "projects/debian-cloud/global/images/family/debian-11"
}

variable "instance_count" {
  description = "Number of GCP instances"
  type        = number
  default     = 3
}

variable "public_ssh_key_path" {
  description = "Path to the public SSH key"
  type        = string
  default     = "~/.ssh/id_rsa.pub"
}
terraform {
  required_providers {
    google = {
      source  = "hashicorp/google"
      version = "6.15.0"
    }
  }
}

provider "google" {
  project = var.project_id
  region  = var.region
  zone    = var.zone
}

# VPC Network
resource "google_compute_network" "nomad_network" {
  name                    = "nomad-network"
  auto_create_subnetworks = false
}

# Subnet
resource "google_compute_subnetwork" "nomad_subnet" {
  name          = "nomad-subnet"
  ip_cidr_range = "10.0.1.0/24"
  network       = google_compute_network.nomad_network.id
  region        = var.region
}

# Firewall Rules
resource "google_compute_firewall" "nomad_firewall" {
  name    = "nomad-firewall"
  network = google_compute_network.nomad_network.id

  allow {
    protocol = "tcp"
    ports    = ["22"] # Just SSH for now
  }

  source_ranges = ["0.0.0.0/0"]
  target_tags   = ["nomad-instance"]
}

# Instance Template
resource "google_compute_instance_template" "nomad_template" {
  name        = "nomad-instance-template"
  machine_type = var.machine_type

  disk {
    source_image = var.image
    boot         = true
    auto_delete  = true
  }

  network_interface {
    subnetwork = google_compute_subnetwork.nomad_subnet.id
    access_config {} # Adds external IP
  }

  metadata = {
    ssh-keys = "nomad-user:${file(var.public_ssh_key_path)}"
  }

  tags = ["nomad-instance"]
}

# Managed Instance Group
resource "google_compute_instance_group_manager" "nomad_instances" {
  name               = "nomad-instance-group"
  base_instance_name = "nomad-instance"

  version {
    instance_template = google_compute_instance_template.nomad_template.id
  }

  target_size        = var.instance_count

  named_port {
    name = "nomad"
    port = 4646
  }
}

# Output Instance Group Information
output "instance_group_name" {
  value = google_compute_instance_group_manager.nomad_instances.name
}

output "instance_template_name" {
  value = google_compute_instance_template.nomad_template.name
}
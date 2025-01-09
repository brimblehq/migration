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
resource "google_compute_network" "brimble_network" {
  name                    = "brimble-network"
}

# Subnet
resource "google_compute_subnetwork" "brimble_subnet" {
  name          = "brimble-subnet"
  ip_cidr_range = "10.0.1.0/24"
  network       = google_compute_network.brimble_network.id
  region        = var.region
}

# Firewall Rules
resource "google_compute_firewall" "brimble_firewall" {
  name    = "brimble-firewall"
  network = google_compute_network.brimble_network.id

  allow {
    protocol = "tcp"
    ports    = ["22"] # Just SSH for now
  }

  source_ranges = ["0.0.0.0/0"]
  target_tags   = ["brimble-instance"]
}

# Instance Template
resource "google_compute_instance_template" "brimble_template" {
  name        = "brimble-instance-template"
  machine_type = var.machine_type

  disk {
    source_image = var.image
    boot         = true
    auto_delete  = true
  }

  network_interface {
    subnetwork = google_compute_subnetwork.brimble_subnet.id
    access_config {} # Adds external IP
  }

  metadata = {
    ssh-keys = "brimble-user:${file(var.public_ssh_key_path)}"
  }

  tags = ["brimble-instance"]
}

# Managed Instance Group
resource "google_compute_instance_group_manager" "brimble_instances" {
  name               = "brimble-instance-group"
  base_instance_name = "brimble-instance"

  version {
    instance_template = google_compute_instance_template.brimble_template.id
  }

  target_size        = var.instance_count

  named_port {
    name = "brimble"
    port = 4646
  }
}
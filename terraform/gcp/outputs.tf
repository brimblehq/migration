output "vpc_name" {
  value = google_compute_network.nomad_network.name
}

output "subnet_name" {
  value = google_compute_subnetwork.nomad_subnet.name
}

output "firewall_name" {
  value = google_compute_firewall.nomad_firewall.name
}

output "instance_group_name" {
  value = google_compute_instance_group_manager.nomad_instances.name
}

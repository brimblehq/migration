output "vpc_name" {
  value = google_compute_network.brimble_network.name
}

output "subnet_name" {
  value = google_compute_subnetwork.brimble_subnet.name
}

output "firewall_name" {
  value = google_compute_firewall.brimble_firewall.name
}

output "instance_group_name" {
  value = google_compute_instance_group_manager.brimble_instances.name
}

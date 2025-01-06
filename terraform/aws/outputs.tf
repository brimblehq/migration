output "vpc_id" {
  description = "ID of the VPC"
  value       = aws_vpc.cluster_vpc.id
}

output "subnet_id" {
  description = "ID of the subnet"
  value       = aws_subnet.brimble_subnet.id
}

output "security_group_id" {
  description = "ID of the security group"
  value       = aws_security_group.brimble_sg.id
}

output "instance_public_ips" {
  description = "Public IPs of the brimble EC2 instances"
  value       = aws_instance.brimble_instance[*].public_ip
}

output "private_key_path" {
  description = "Path to the generated private key file"
  value       = local_file.brimble_key.filename
}

output "key_pair_name" {
  description = "Name of the AWS key pair"
  value       = aws_key_pair.brimble-key.key_name
}
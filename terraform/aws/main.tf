terraform {
  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 5.0"
    }
  }
}

# Configure the AWS Provider
provider "aws" {
  region = "us-east-1"
}

# Cluster VPC
resource "aws_vpc" "cluster_vpc" {
  cidr_block = "10.0.0.0/16"
  enable_dns_support   = true
  enable_dns_hostnames = true
  tags = {
    Name = "nomad-vpc"
  }
}


# Subnet
resource "aws_subnet" "nomad_subnet" {
  vpc_id            = aws_vpc.cluster_vpc.id
  cidr_block        = "10.0.1.0/24"
  map_public_ip_on_launch = true
  availability_zone = "us-east-1a"
  tags = {
    Name = "nomad-subnet"
  }
}

# Internet Gateway
resource "aws_internet_gateway" "nomad_igw" {
  vpc_id = aws_vpc.cluster_vpc.id
  tags = {
    Name = "nomad-igw"
  }
}


# Route Table
resource "aws_route_table" "nomad_route_table" {
  vpc_id = aws_vpc.cluster_vpc.id

  route {
    cidr_block = "0.0.0.0/0"
    gateway_id = aws_internet_gateway.nomad_igw.id
  }

  tags = {
    Name = "nomad-route-table"
  }
}

# Associate Route Table with Subnet
resource "aws_route_table_association" "nomad_subnet_assoc" {
  subnet_id      = aws_subnet.nomad_subnet.id
  route_table_id = aws_route_table.nomad_route_table.id
}

# Security Group
resource "aws_security_group" "nomad_sg" {
  vpc_id = aws_vpc.cluster_vpc.id
  tags = {
    Name = "nomad-sg"
  }
}

# Ingress Rules
resource "aws_vpc_security_group_ingress_rule" "ssh_ingress" {
  security_group_id = aws_security_group.nomad_sg.id
  from_port         = 22
  to_port           = 22
  ip_protocol       = "tcp"
  cidr_ipv4         = "0.0.0.0/0"
}

resource "aws_vpc_security_group_ingress_rule" "all_tcp_ingress" { # Let's talk about these specifics
  security_group_id = aws_security_group.nomad_sg.id
  from_port         = 0
  to_port           = 65535
  ip_protocol       = "tcp"
  cidr_ipv4         = "0.0.0.0/0"
}

# Egress Rules
resource "aws_vpc_security_group_egress_rule" "all_egress" {
  security_group_id = aws_security_group.nomad_sg.id
  from_port         = 0
  to_port           = 0
  ip_protocol       = "-1"
  cidr_ipv4         = "0.0.0.0/0"
}

# EC2 Instances
resource "aws_instance" "nomad_instance" {
  count = var.instance_count

  ami           = "ami-0e2c8caa4b6378d8c" # Ubuntu
  instance_type = "t3.medium"

  subnet_id                   = aws_subnet.nomad_subnet.id
  vpc_security_group_ids      = [aws_security_group.nomad_sg.id]
  associate_public_ip_address = true

  tags = {
    Name = "nomad-instance-${count.index + 1}"
  }
}

resource "tls_private_key" "pk" {
  algorithm = "RSA"
  rsa_bits  = 4096
}

resource "aws_key_pair" "nomad-key" {
  key_name   = "nomad-aws-key-pair"
  public_key = tls_private_key.pk.public_key_openssh
}

resource "local_file" "nomad_key" {
  content         = tls_private_key.pk.private_key_pem
  filename        = "./nomad-aws-key-pair.pem"
  file_permission = "0400"
}
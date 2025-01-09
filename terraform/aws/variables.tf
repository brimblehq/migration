variable "instance_count" {
  description = "Number of AWS instances to launch"
  type        = number
  default     = 1
}

variable "region" {
  description = "AWS region to deploy resources"
  type        = string
  default     = "us-east-1"
}

variable "vpc_cidr_block" {
  description = "CIDR block for the VPC"
  type        = string
  default     = "10.0.0.0/16"
}

variable "subnet_cidr_block" {
  description = "CIDR block for the public subnet"
  type        = string
  default     = "10.0.1.0/24"
}

variable "availability_zone" {
  description = "Availability zone for the subnet"
  type        = string
  default     = "us-east-1a"
}

variable "ami" {
  description = "AMI ID for the EC2 instances"
  type        = string
  default     = "ami-0e2c8caa4b6378d8c"
}

variable "instance_type" {
  description = "Instance type for EC2 instances"
  type        = string
  default     = "t2.large"
}
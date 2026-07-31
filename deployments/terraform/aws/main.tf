terraform {
  required_version = ">= 1.5.0"
  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 5.0"
    }
  }
}

provider "aws" {
  region = var.aws_region

  default_tags {
    tags = {
      Project     = "HypasisSyncProtocol"
      Environment = var.environment
      ManagedBy   = "Terraform"
    }
  }
}

# 1. VPC & Networking
resource "aws_vpc" "hypasis_vpc" {
  cidr_block           = "10.0.0.0/16"
  enable_dns_hostnames = true
  enable_dns_support   = true

  tags = {
    Name = "hypasis-vpc-${var.environment}"
  }
}

# Internet Gateway
resource "aws_internet_gateway" "gw" {
  vpc_id = aws_vpc.hypasis_vpc.id

  tags = {
    Name = "hypasis-igw-${var.environment}"
  }
}

# Public Subnets (2 Availability Zones for HA)
resource "aws_subnet" "public_az1" {
  vpc_id                  = aws_vpc.hypasis_vpc.id
  cidr_block              = "10.0.1.0/24"
  availability_zone       = "${var.aws_region}a"
  map_public_ip_on_launch = true

  tags = {
    Name = "hypasis-public-az1-${var.environment}"
  }
}

resource "aws_subnet" "public_az2" {
  vpc_id                  = aws_vpc.hypasis_vpc.id
  cidr_block              = "10.0.2.0/24"
  availability_zone       = "${var.aws_region}b"
  map_public_ip_on_launch = true

  tags = {
    Name = "hypasis-public-az2-${var.environment}"
  }
}

# Route Table for Public Subnets
resource "aws_route_table" "public_rt" {
  vpc_id = aws_vpc.hypasis_vpc.id

  route {
    cidr_block = "0.0.0.0/0"
    gateway_id = aws_internet_gateway.gw.id
  }

  tags = {
    Name = "hypasis-public-rt-${var.environment}"
  }
}

resource "aws_route_table_association" "a" {
  subnet_id      = aws_subnet.public_az1.id
  route_table_id = aws_route_table.public_rt.id
}

resource "aws_route_table_association" "b" {
  subnet_id      = aws_subnet.public_az2.id
  route_table_id = aws_route_table.public_rt.id
}

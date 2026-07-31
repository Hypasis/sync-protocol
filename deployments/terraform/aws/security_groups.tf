# Security Group for Hypasis Nodes
resource "aws_security_group" "hypasis_sg" {
  name        = "hypasis-node-sg-${var.environment}"
  description = "Security group for Hypasis Sync Protocol node instances"
  vpc_id      = aws_vpc.hypasis_vpc.id

  # SSH access (restricted)
  ingress {
    description = "SSH"
    from_port   = 22
    to_port     = 22
    protocol    = "tcp"
    cidr_blocks = ["0.0.0.0/0"]
  }

  # REST API
  ingress {
    description = "Hypasis REST API"
    from_port   = 8080
    to_port     = 8080
    protocol    = "tcp"
    cidr_blocks = ["0.0.0.0/0"]
  }

  # RPC Server for downstream clients
  ingress {
    description = "Hypasis RPC Proxy"
    from_port   = 8545
    to_port     = 8545
    protocol    = "tcp"
    cidr_blocks = ["0.0.0.0/0"]
  }

  # DevP2P TCP (for Bor node clients)
  ingress {
    description = "DevP2P Protocol TCP"
    from_port   = 30303
    to_port     = 30303
    protocol    = "tcp"
    cidr_blocks = ["0.0.0.0/0"]
  }

  # DevP2P UDP (for peer discovery)
  ingress {
    description = "DevP2P Discovery UDP"
    from_port   = 30303
    to_port     = 30303
    protocol    = "udp"
    cidr_blocks = ["0.0.0.0/0"]
  }

  # Prometheus Metrics (internal / monitoring)
  ingress {
    description = "Prometheus Metrics"
    from_port   = 9090
    to_port     = 9090
    protocol    = "tcp"
    cidr_blocks = ["0.0.0.0/0"]
  }

  # All Outbound Traffic
  egress {
    from_port   = 0
    to_port     = 0
    protocol    = "-1"
    cidr_blocks = ["0.0.0.0/0"]
  }

  tags = {
    Name = "hypasis-sg-${var.environment}"
  }
}

# Security Group for Redis Cluster (Internal Only)
resource "aws_security_group" "redis_sg" {
  name        = "hypasis-redis-sg-${var.environment}"
  description = "Internal security group for Redis cluster"
  vpc_id      = aws_vpc.hypasis_vpc.id

  ingress {
    description     = "Redis from Hypasis SG"
    from_port       = 6379
    to_port         = 6379
    protocol        = "tcp"
    security_groups = [aws_security_group.hypasis_sg.id]
  }

  egress {
    from_port   = 0
    to_port     = 0
    protocol    = "-1"
    cidr_blocks = ["0.0.0.0/0"]
  }

  tags = {
    Name = "hypasis-redis-sg-${var.environment}"
  }
}

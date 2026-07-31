# Find latest Amazon Linux 2023 ARM64 AMI
data "aws_ami" "amazon_linux_arm64" {
  most_recent = true
  owners      = ["amazon"]

  filter {
    name   = "name"
    values = ["al2023-ami-2023.*-arm64"]
  }

  filter {
    name   = "virtualization-type"
    values = ["hvm"]
  }
}

# Launch Template with GP3 EBS & UserData Bootstrap
resource "aws_launch_template" "hypasis_lt" {
  name_prefix   = "hypasis-lt-${var.environment}-"
  image_id      = data.aws_ami.amazon_linux_arm64.id
  instance_type = var.instance_type

  network_interfaces {
    associate_public_ip_address = true
    security_groups             = [aws_security_group.hypasis_sg.id]
  }

  # Cost-optimized GP3 EBS storage
  block_device_mappings {
    device_name = "/dev/xvda"

    ebs {
      volume_size           = var.ebs_volume_size
      volume_type           = "gp3"
      iops                  = 3000
      throughput            = 125
      delete_on_termination = true
      encrypted             = true
    }
  }

  user_data = base64encode(<<-EOF
    #!/bin/bash
    sudo dnf update -y
    sudo dnf install -y docker git
    sudo systemctl enable --now docker
    sudo usermod -aG docker ec2-user

    # Install Docker Compose
    sudo mkdir -p /usr/local/lib/docker/cli-plugins
    sudo curl -SL https://github.com/docker/compose/releases/latest/download/docker-compose-linux-aarch64 -o /usr/local/lib/docker/cli-plugins/docker-compose
    sudo chmod +x /usr/local/lib/docker/cli-plugins/docker-compose

    # Clone repository and start service
    mkdir -p /app && cd /app
    git clone https://github.com/Hypasis/sync-protocol.git .
    
    cat << 'ENVFILE' > .env
    ETH_L1_RPC_URL=${var.eth_l1_rpc_url}
    POLYGON_RPC_1=${var.polygon_rpc_url}
    REDIS_URL=redis://${aws_elasticache_cluster.redis.cache_nodes[0].address}:6379
    ENVFILE

    docker compose -f docker-compose.cloud.yaml up -d
  EOF
  )

  tag_specifications {
    resource_type = "instance"
    tags = {
      Name = "hypasis-node-${var.environment}"
    }
  }
}

# Auto Scaling Group mixing Spot & On-Demand for 60-70% Cost Savings
resource "aws_autoscaling_group" "hypasis_asg" {
  name_prefix         = "hypasis-asg-${var.environment}-"
  vpc_zone_identifier = [aws_subnet.public_az1.id, aws_subnet.public_az2.id]
  min_size            = var.min_cluster_size
  max_size            = var.max_cluster_size
  desired_capacity    = var.min_cluster_size

  mixed_instances_policy {
    instances_distribution {
      on_demand_base_capacity                  = 1
      on_demand_percentage_above_base_capacity = 100 - var.spot_price_percentage
      spot_allocation_strategy                 = "lowest-price"
    }

    launch_template {
      launch_template_specification {
        launch_template_id = aws_launch_template.hypasis_lt.id
        version            = "$Latest"
      }

      override {
        instance_type = var.instance_type
      }
      override {
        instance_type = "c7g.xlarge"
      }
    }
  }

  lifecycle {
    create_before_destroy = true
  }
}

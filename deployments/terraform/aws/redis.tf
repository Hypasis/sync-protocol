# Subnet Group for ElastiCache
resource "aws_elasticache_subnet_group" "redis_subnets" {
  name       = "hypasis-redis-subnets-${var.environment}"
  subnet_ids = [aws_subnet.public_az1.id, aws_subnet.public_az2.id]
}

# Cost-Optimized ElastiCache Redis Cluster (cache.t4g.small)
resource "aws_elasticache_cluster" "redis" {
  cluster_id           = "hypasis-redis-${var.environment}"
  engine               = "redis"
  node_type            = var.redis_node_type
  num_cache_nodes      = 1
  parameter_group_name = "default.redis7"
  engine_version       = "7.0"
  port                 = 6379
  subnet_group_name    = aws_elasticache_subnet_group.redis_subnets.name
  security_group_ids   = [aws_security_group.redis_sg.id]

  tags = {
    Name = "hypasis-redis-${var.environment}"
  }
}

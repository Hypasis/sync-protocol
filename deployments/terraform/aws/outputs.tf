output "vpc_id" {
  description = "The ID of the VPC"
  value       = aws_vpc.hypasis_vpc.id
}

output "security_group_id" {
  description = "Security Group ID for Hypasis nodes"
  value       = aws_security_group.hypasis_sg.id
}

output "redis_endpoint" {
  description = "ElastiCache Redis endpoint address"
  value       = aws_elasticache_cluster.redis.cache_nodes[0].address
}

output "devp2p_connection_info" {
  description = "DevP2P connection guide for Bor node clients"
  value       = "Add node IPs on Port 30303 to your Bor client --bootnodes flag."
}

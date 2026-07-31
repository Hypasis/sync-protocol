variable "aws_region" {
  description = "AWS region for deployment"
  type        = string
  default     = "us-east-1"
}

variable "environment" {
  description = "Environment name (e.g. prod, staging, dev)"
  type        = string
  default     = "prod"
}

variable "instance_type" {
  description = "EC2 instance type for Hypasis nodes (Graviton3 ARM64 recommended for 40% cost savings)"
  type        = string
  default     = "t4g.xlarge"
}

variable "ebs_volume_size" {
  description = "EBS GP3 volume size in GB per node"
  type        = number
  default     = 200
}

variable "min_cluster_size" {
  description = "Minimum number of Hypasis instances in Auto Scaling Group"
  type        = number
  default     = 2
}

variable "max_cluster_size" {
  description = "Maximum number of Hypasis instances in Auto Scaling Group"
  type        = number
  default     = 5
}

variable "spot_price_percentage" {
  description = "Percentage of spot instances vs on-demand in Auto Scaling Group (70 = 70% spot / 30% on-demand)"
  type        = number
  default     = 70
}

variable "redis_node_type" {
  description = "ElastiCache Redis node type (t4g.small for cost optimization)"
  type        = string
  default     = "cache.t4g.small"
}

variable "eth_l1_rpc_url" {
  description = "Ethereum L1 RPC URL for RootChain checkpoint verification"
  type        = string
  default     = "https://eth-mainnet.g.alchemy.com/v2/YOUR_KEY"
}

variable "polygon_rpc_url" {
  description = "Polygon PoS Upstream RPC URL for block fetching"
  type        = string
  default     = "https://polygon-rpc.com"
}

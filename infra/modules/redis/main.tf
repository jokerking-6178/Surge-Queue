# OCI Cache (Redis-compatible) module — backing store for the queue engine.
# OCI Cache is a managed Redis service. The queue engine connects to this
# instead of running a self-managed Redis pod.

variable "compartment" {
  type = string
}

variable "redis_name" {
  type = string
}

variable "subnet_id" {
  type = string
}

variable "capacity_gbps" {
  type    = number
  default = 1
}

variable "tags" {
  type = map(string)
}

# OCI Cache with Redis
resource "oci_redis_redis_cluster" "this" {
  compartment_id  = var.compartment
  display_name    = var.redis_name
  node_count      = 1
  software_version = "REDIS_7_0"
  subnet_id       = var.subnet_id

  # Capacity (throughput) — 1 = lower, 2 = higher
  cluster_mode {
    shard_count = 1
    replicas_per_node_count = 0
  }

  nsg_ids = []
  freeform_tags = var.tags
}

output "redis_endpoint" {
  value = oci_redis_redis_cluster.this.nodes[0].private_ip
}

output "redis_port" {
  value = 6379
}

output "redis_cluster_id" {
  value = oci_redis_redis_cluster.this.id
}

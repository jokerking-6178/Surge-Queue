# ---- OCI connection ----
variable "tenancy_ocid" {
  type        = string
  description = "OCI Tenancy OCID"
}

variable "user_ocid" {
  type        = string
  description = "OCI User OCID"
}

variable "fingerprint" {
  type        = string
  description = "API key fingerprint"
}

variable "private_key_path" {
  type        = string
  description = "Path to the API private key"
}

variable "region" {
  type        = string
  default     = "ap-mumbai-1"
  description = "OCI region (ap-mumbai-1 = Mumbai, ap-hyderabad-1 = Hyderabad)"
}

variable "compartment_ocid" {
  type        = string
  description = "Compartment OCID where resources will be created"
}

# ---- Cluster ----
variable "cluster_name" {
  type    = string
  default = "surge-queue-cluster"
}

variable "k8s_version" {
  type    = string
  default = "v1.29.1"
}

variable "node_pool_size" {
  type    = string
  default = "VM.Standard3.Flex"
  description = "Shape for OKE worker nodes. VM.Standard3.Flex is flexible (configurable CPU/mem)."
}

variable "node_pool_ocpus" {
  type    = number
  default = 2
}

variable "node_pool_memory_gb" {
  type    = number
  default = 16
}

variable "node_pool_min_nodes" {
  type    = number
  default = 3
}

variable "node_pool_max_nodes" {
  type    = number
  default = 10
}

# ---- Redis ----
variable "redis_name" {
  type    = string
  default = "surge-redis"
}

variable "redis_capacity_gbps" {
  type    = number
  default = 1
  description = "OCI Cache capacity: 1 (lower) or 2 (higher throughput)"
}

# ---- Tags ----
variable "environment" {
  type    = string
  default = "dev"
}

variable "project" {
  type    = string
  default = "surge-queue"
}

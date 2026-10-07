provider "oci" {
  tenancy_ocid     = var.tenancy_ocid
  user_ocid        = var.user_ocid
  fingerprint      = var.fingerprint
  private_key_path = var.private_key_path
  region           = var.region
}

# Common tags applied to all resources
locals {
  tags = {
    "project"     = var.project
    "environment" = var.environment
    "managed-by"  = "terraform"
  }
}

# ============================================================
# VCN — Virtual Cloud Network with subnets for OKE
# ============================================================
module "vcn" {
  source       = "./modules/vcn"
  compartment  = var.compartment_ocid
  vcn_name     = "${var.project}-vcn"
  vcn_cidr     = "10.0.0.0/16"
  tags         = local.tags
}

# ============================================================
# OKE — Oracle Kubernetes Engine cluster + node pool
# ============================================================
module "oke" {
  source             = "./modules/oke"
  compartment        = var.compartment_ocid
  cluster_name       = var.cluster_name
  k8s_version        = var.k8s_version
  vcn_id             = module.vcn.vcn_id
  service_subnet_id  = module.vcn.service_subnet_id
  nodepool_subnet_id = module.vcn.nodepool_subnet_id
  lb_subnet_id       = module.vcn.lb_subnet_id
  node_shape         = var.node_pool_size
  node_ocpus         = var.node_pool_ocpus
  node_memory_gb     = var.node_pool_memory_gb
  min_nodes          = var.node_pool_min_nodes
  max_nodes          = var.node_pool_max_nodes
  tags               = local.tags
}

# ============================================================
# OCI Cache (Redis-compatible) — backing store for the queue engine
# ============================================================
module "redis" {
  source        = "./modules/redis"
  compartment   = var.compartment_ocid
  redis_name    = var.redis_name
  subnet_id     = module.vcn.nodepool_subnet_id
  capacity_gbps = var.redis_capacity_gbps
  tags          = local.tags
}

# ============================================================
# OCI Container Registry — repository for our Docker images
# ============================================================
module "ocr" {
  source       = "./modules/ocr"
  tenancy_ocid = var.tenancy_ocid
  repos = {
    "queue-engine" = "Go surge queue engine"
    "backend"      = "Java Spring Boot backend"
    "loadtest"     = "Go load tester"
  }
  tags = local.tags
}

# ============================================================
# KMS Vault — store the JWT secret for ExternalSecrets to sync into K8s
# ============================================================
resource "random_password" "jwt_secret" {
  length  = 64
  special = false
}

resource "oci_kms_vault" "surge_vault" {
  compartment_id = var.compartment_ocid
  display_name   = "${var.project}-vault"
  vault_type     = "DEFAULT"
  freeform_tags  = local.tags
}

resource "oci_kms_key" "jwt_key" {
  compartment_id      = var.compartment_ocid
  display_name        = "${var.project}-jwt-key"
  management_endpoint = oci_kms_vault.surge_vault.management_endpoint
  freeform_tags       = local.tags

  key_shape {
    algorithm = "AES"
    length    = 32
  }
}

resource "oci_vault_secret" "jwt_secret" {
  compartment_id = var.compartment_ocid
  secret_name    = "${var.project}-jwt-secret"
  description    = "JWT signing secret shared between queue engine and backend"
  vault_id       = oci_kms_vault.surge_vault.id
  key_id         = oci_kms_key.jwt_key.id

  secret_content {
    content_type = "BASE64"
    content      = base64encode(random_password.jwt_secret.result)
  }
}

# ============================================================
# Outputs
# ============================================================
output "oke_cluster_id" {
  value = module.oke.cluster_id
}

output "redis_endpoint" {
  value     = module.redis.redis_endpoint
  sensitive = true
}

output "ocr_repos" {
  value = module.ocr.repo_names
}

output "vault_secret_ocid" {
  value = oci_vault_secret.jwt_secret.id
}

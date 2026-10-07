# OCI Container Registry module — creates repos for our Docker images.
# OCR repos are per-tenancy, created in the root compartment.

variable "tenancy_ocid" {
  type = string
}

variable "repos" {
  type = map(string)
  description = "Map of repo name -> description"
}

variable "tags" {
  type = map(string)
}

locals {
  region_prefix = "ap-mumbai-1" # adjust to match var.region in root
  namespace    = "surge"
}

resource "oci_artifacts_container_repository" "repos" {
  for_each = var.repos

  compartment_id  = var.tenancy_ocid
  display_name    = "${local.namespace}/${each.key}"
  repository_type = "PRIVATE"
  is_immutable    = false
  readme          = each.value
  freeform_tags   = var.tags
}

output "repo_names" {
  value = { for k, v in oci_artifacts_container_repository.repos : k => v.display_name }
}

output "repo_ids" {
  value = { for k, v in oci_artifacts_container_repository.repos : k => v.id }
}

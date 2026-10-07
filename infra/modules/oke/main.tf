# OKE module — creates the Kubernetes cluster and a node pool

variable "compartment" {
  type = string
}

variable "cluster_name" {
  type = string
}

variable "k8s_version" {
  type = string
}

variable "vcn_id" {
  type = string
}

variable "service_subnet_id" {
  type = string
}

variable "nodepool_subnet_id" {
  type = string
}

variable "lb_subnet_id" {
  type = string
}

variable "node_shape" {
  type = string
}

variable "node_ocpus" {
  type = number
}

variable "node_memory_gb" {
  type = number
}

variable "min_nodes" {
  type = number
}

variable "max_nodes" {
  type = number
}

variable "tags" {
  type = map(string)
}

# ---- OKE Cluster ----
resource "oci_containerengine_cluster" "this" {
  compartment_id     = var.compartment
  name               = var.cluster_name
  kubernetes_version = var.k8s_version
  vcn_id             = var.vcn_id

  endpoint_config {
    is_public_ip_enabled = true
    subnet_id            = var.lb_subnet_id
  }

  options {
    add_ons {
      is_kubernetes_dashboard_enabled = true
      is_tiller_enabled               = false
    }
    service_lb_subnet_ids = [var.lb_subnet_id]

    admission_controller_options {
      is_pod_security_policy_enabled = false
    }
  }

  cluster_pod_network_options {
    cni_type = "OCI_VCN_IP_NATIVE"
    max_pods_per_node = 31
  }

  freeform_tags = var.tags
}

# ---- Node Pool ----
resource "oci_containerengine_node_pool" "this" {
  compartment_id     = var.compartment
  cluster_id          = oci_containerengine_cluster.this.id
  name               = "${var.cluster_name}-pool"
  kubernetes_version = var.k8s_version
  node_shape          = var.node_shape
  ssh_public_key      = ""

  node_config_details {
    placement_configs {
      availability_domain = data.oci_identity_availability_domains.ads.availability_domains[0].name
      capacity_reservation_id = null
      preemption_action {
        type = "TERMINATE"
      }
      subnet_id = var.nodepool_subnet_id
    }
    size = var.min_nodes

    node_pool_pod_network_option_details {
      cni_type       = "OCI_VCN_IP_NATIVE"
      max_nsgs_per_pod = 0
      max_pods_per_node = 31
    }
  }

  node_shape_config {
    ocpus         = var.node_ocpus
    memory_in_gbs = var.node_memory_gb
  }

  freeform_tags = var.tags
}

# Fetch ADs in the region
data "oci_identity_availability_domains" "ads" {
  compartment_id = var.compartment
}

# ---- Outputs ----
output "cluster_id" {
  value = oci_containerengine_cluster.this.id
}

output "cluster_endpoint" {
  value = oci_containerengine_cluster.this.endpoints[0].public_endpoint
}

output "node_pool_id" {
  value = oci_containerengine_node_pool.this.id
}

# VCN module — creates VCN, subnets, gateways, and security lists for OKE

variable "compartment" {
  type = string
}

variable "vcn_name" {
  type = string
}

variable "vcn_cidr" {
  type = string
}

variable "tags" {
  type = map(string)
}

# ---- VCN ----
resource "oci_core_vcn" "this" {
  compartment_id = var.compartment
  display_name   = var.vcn_name
  cidr_block     = var.vcn_cidr
  dns_label      = "surge"
  freeform_tags  = var.tags
}

# ---- Internet Gateway ----
resource "oci_core_internet_gateway" "ig" {
  compartment_id = var.compartment
  vcn_id         = oci_core_vcn.this.id
  display_name   = "${var.vcn_name}-ig"
  freeform_tags  = var.tags
}

# ---- NAT Gateway (for private nodes to pull images) ----
resource "oci_core_nat_gateway" "nat" {
  compartment_id = var.compartment
  vcn_id         = oci_core_vcn.this.id
  display_name   = "${var.vcn_name}-nat"
  freeform_tags  = var.tags
}

# ---- Service Gateway (for OCI services like Object Storage) ----
resource "oci_core_service_gateway" "sgw" {
  compartment_id = var.compartment
  vcn_id         = oci_core_vcn.this.id
  display_name   = "${var.vcn_name}-sgw"
  services {
    service_id = data.oci_core_services.all.services[0].id
  }
  freeform_tags = var.tags
}

data "oci_core_services" "all" {
  filter {
    name   = "name"
    values = ["All .* Services In Region Services"]
  }
}

# ---- Route Tables ----
# Public route table (via IGW)
resource "oci_core_route_table" "public" {
  compartment_id = var.compartment
  vcn_id         = oci_core_vcn.this.id
  display_name   = "${var.vcn_name}-public-rt"

  route_rules {
    destination       = "0.0.0.0/0"
    destination_type = "CIDR_BLOCK"
    network_entity_id = oci_core_internet_gateway.ig.id
  }
  freeform_tags = var.tags
}

# Private route table (via NAT + SGW)
resource "oci_core_route_table" "private" {
  compartment_id = var.compartment
  vcn_id         = oci_core_vcn.this.id
  display_name   = "${var.vcn_name}-private-rt"

  route_rules {
    destination       = "0.0.0.0/0"
    destination_type = "CIDR_BLOCK"
    network_entity_id = oci_core_nat_gateway.nat.id
  }
  route_rules {
    destination       = data.oci_core_services.all.services[0].cidr_block
    destination_type = "SERVICE_CIDR_BLOCK"
    network_entity_id = oci_core_service_gateway.sgw.id
  }
  freeform_tags = var.tags
}

# ---- Subnets ----

# Public LB subnet
resource "oci_core_subnet" "lb" {
  compartment_id    = var.compartment
  vcn_id            = oci_core_vcn.this.id
  cidr_block        = cidrsubnet(var.vcn_cidr, 4, 0) # 10.0.0.0/20
  display_name      = "${var.vcn_name}-lb-subnet"
  route_table_id    = oci_core_route_table.public.id
  security_list_ids = [oci_core_security_list.lb.id]
  dns_label         = "lb"
  freeform_tags     = var.tags
}

# Node pool subnet (private)
resource "oci_core_subnet" "nodepool" {
  compartment_id    = var.compartment
  vcn_id            = oci_core_vcn.this.id
  cidr_block        = cidrsubnet(var.vcn_cidr, 4, 1) # 10.0.16.0/20
  display_name      = "${var.vcn_name}-node-subnet"
  route_table_id    = oci_core_route_table.private.id
  security_list_ids = [oci_core_security_list.nodes.id]
  dns_label         = "nodes"
  prohibit_public_ip_on_vnic = true
  freeform_tags     = var.tags
}

# K8s service (cluster) subnet
resource "oci_core_subnet" "service" {
  compartment_id    = var.compartment
  vcn_id            = oci_core_vcn.this.id
  cidr_block        = cidrsubnet(var.vcn_cidr, 4, 2) # 10.0.32.0/20
  display_name      = "${var.vcn_name}-svc-subnet"
  route_table_id    = oci_core_route_table.private.id
  security_list_ids = [oci_core_security_list.nodes.id]
  dns_label         = "svc"
  prohibit_public_ip_on_vnic = true
  freeform_tags     = var.tags
}

# ---- Security Lists ----

# LB security list — allow 80/443 from anywhere, all to nodes
resource "oci_core_security_list" "lb" {
  compartment_id = var.compartment
  vcn_id         = oci_core_vcn.this.id
  display_name   = "${var.vcn_name}-lb-sl"
  freeform_tags  = var.tags

  egress_security_rules {
    destination = "0.0.0.0/0"
    protocol    = "all"
  }

  ingress_security_rules {
    protocol = "6" # TCP
    source   = "0.0.0.0/0"
    tcp_options {
      min = 80
      max = 80
    }
  }
  ingress_security_rules {
    protocol = "6"
    source   = "0.0.0.0/0"
    tcp_options {
      min = 443
      max = 443
    }
  }
}

# Node security list
resource "oci_core_security_list" "nodes" {
  compartment_id = var.compartment
  vcn_id         = oci_core_vcn.this.id
  display_name   = "${var.vcn_name}-node-sl"
  freeform_tags  = var.tags

  egress_security_rules {
    destination = "0.0.0.0/0"
    protocol    = "all"
  }

  # Allow traffic from within VCN
  ingress_security_rules {
    protocol = "all"
    source   = var.vcn_cidr
  }

  # Allow LB traffic to node ports
  ingress_security_rules {
    protocol = "6"
    source   = oci_core_subnet.lb.cidr_block
    tcp_options {
      min = 30000
      max = 32767
    }
  }
}

# ---- Outputs ----
output "vcn_id" {
  value = oci_core_vcn.this.id
}

output "lb_subnet_id" {
  value = oci_core_subnet.lb.id
}

output "nodepool_subnet_id" {
  value = oci_core_subnet.nodepool.id
}

output "service_subnet_id" {
  value = oci_core_subnet.service.id
}

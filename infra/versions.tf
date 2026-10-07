terraform {
  required_version = ">= 1.5.0"

  required_providers {
    oci = {
      source  = "oracle/oci"
      version = ">= 5.30.0"
    }
    random = {
      source  = "hashicorp/random"
      version = ">= 3.6.0"
    }
  }

  # Uncomment and configure for remote state in OCI Object Storage
  # backend "s3" {
  #   endpoint   = "https://<namespace>.compat.objectstorage.<region>.oraclecloud.com"
  #   bucket     = "surge-tfstate"
  #   key        = "surge/terraform.tfstate"
  #   region     = "ap-mumbai-1"
  #   force_path_style = true
  # }
}

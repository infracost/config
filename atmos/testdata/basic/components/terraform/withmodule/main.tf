terraform {
  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = ">= 5.0.0"
    }
  }
}

variable "bucket_name" {
  type    = string
  default = "default-bucket"
}

module "shared" {
  source      = "../shared"
  bucket_name = var.bucket_name
}

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
  default = "shared"
}

resource "aws_s3_bucket" "shared" {
  bucket = var.bucket_name
}

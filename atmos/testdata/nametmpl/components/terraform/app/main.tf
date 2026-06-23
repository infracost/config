terraform {
  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = ">= 5.0.0"
    }
  }
}

variable "resource_name" {
  type    = string
  default = ""
}

resource "aws_instance" "this" {
  tags = {
    Name = var.resource_name
  }
}

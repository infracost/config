terraform {
  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = ">= 5.0.0"
    }
  }
}

variable "name" {
  type    = string
  default = ""
}

variable "instance_type" {
  type    = string
  default = "t3.nano"
}

resource "aws_instance" "this" {
  instance_type = var.instance_type
  tags = {
    Name = var.name
  }
}

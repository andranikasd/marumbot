# Provider and state configuration.
#
# State lives in an R2 bucket through the S3-compatible API. That bucket is the
# one piece of infrastructure Terraform cannot create for itself, so it is made
# once by hand -- see README.md. Everything else in this directory is
# reproducible from an empty account.
#
# The backend is deliberately left unconfigured here: each environment passes
# its own key with `-backend-config=envs/<env>.backend.hcl`, so dev and
# production can never write to the same state file by accident.

terraform {
  required_version = ">= 1.9.0"

  required_providers {
    cloudflare = {
      source  = "cloudflare/cloudflare"
      version = "~> 5.24"
    }
  }

  # No state locking: R2's S3 API offers no DynamoDB-style lock, so two
  # concurrent applies can corrupt state. Single operator, one apply at a
  # time -- and CI must never run apply in parallel with a human.
  backend "s3" {}
}

provider "cloudflare" {
  api_token = var.cloudflare_api_token
}

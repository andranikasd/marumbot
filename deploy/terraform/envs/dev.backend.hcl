# State location for dev. Passed with:
#     terraform init -backend-config=envs/dev.backend.hcl
#
# R2 speaks the S3 API, so the s3 backend works against it. The checksum and
# region flags are required because R2 implements a subset of S3 and Terraform
# would otherwise send headers R2 rejects.

bucket = "marum-terraform-state"
key    = "dev/terraform.tfstate"

region                      = "auto"
skip_credentials_validation = true
skip_metadata_api_check     = true
skip_region_validation      = true
skip_requesting_account_id  = true
skip_s3_checksum            = true
use_path_style              = true

# endpoints.s3 is supplied at init time, because it embeds the account ID:
#     -backend-config="endpoints={s3=\"https://<account>.r2.cloudflarestorage.com\"}"

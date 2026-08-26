# Marum's Cloudflare infrastructure.
#
# Scope: what must exist before a deploy can succeed, and what a deploy must
# never create for itself.
#
#   Terraform owns   Hyperdrive configs, the backup bucket, zone security
#   wrangler owns    the Worker, its routes, its DNS record, its secrets
#   a person owns    the Neon project, the R2 state bucket, API tokens
#
# The split is not arbitrary. A Worker deploy happens on every merge and must
# stay fast and reversible; infrastructure changes are rare, reviewed, and
# planned before they are applied. Keeping them in one pipeline makes the
# common case slow and the rare case invisible.
#
# In particular Terraform does NOT create the DNS record for the bot hostname.
# `custom_domain = true` in wrangler.toml makes Cloudflare create that record,
# and Cloudflare refuses a Custom Domain on a hostname that already carries a
# CNAME -- so a record made here would break the deploy it was meant to enable.

locals {
  name = var.environment == "production" ? "marum" : "marum-${var.environment}"

  # Production must never serve a cached balance. See query_cache_max_age.
  cache_disabled = var.environment == "production" || var.query_cache_max_age == 0

  # One zone, two environments: only production may write zone-wide settings.
  manages_zone = var.environment == "production"
}

# ---------------------------------------------------------------------------
# Hyperdrive: the pooler in front of the origin database.
#
# Workers have no persistent connections and can start anywhere in the world,
# so every isolate would otherwise open its own TCP and TLS handshake to a
# database that is in one region. Hyperdrive holds the pool on Cloudflare's
# side and reuses it, which is what makes Postgres viable from the edge at all.
#
# Its ID goes into wrangler.toml as the HYPERDRIVE binding. That value is not a
# secret -- it is meaningless without the account -- so it is a plain output.
# ---------------------------------------------------------------------------

resource "cloudflare_hyperdrive_config" "postgres" {
  account_id = var.cloudflare_account_id
  name       = "${local.name}-postgres"

  origin = {
    scheme   = "postgres"
    host     = var.database.host
    port     = var.database.port
    database = var.database.name
    user     = var.database.user
    password = var.database.password
  }

  origin_connection_limit = var.origin_connection_limit

  caching = {
    disabled = local.cache_disabled
    max_age  = local.cache_disabled ? null : var.query_cache_max_age
  }
}

# ---------------------------------------------------------------------------
# Backups.
#
# Neon keeps its own point-in-time history, but that history lives in the same
# account as the database. A dump in R2 survives losing that account, and R2
# has no egress charge, so restoring costs nothing at the moment it is most
# needed. Object Lock is not set: a lock that outlives a mistake also outlives
# an erasure request, and account deletion has to be honoured.
# ---------------------------------------------------------------------------

resource "cloudflare_r2_bucket" "backups" {
  account_id    = var.cloudflare_account_id
  name          = "${local.name}-backups"
  location      = "EEUR"
  storage_class = "Standard"
}

resource "cloudflare_r2_bucket_lifecycle" "backups" {
  account_id  = var.cloudflare_account_id
  bucket_name = cloudflare_r2_bucket.backups.name

  rules = [{
    id      = "expire-dumps"
    enabled = true

    conditions = {
      prefix = "dumps/"
    }

    delete_objects_transition = {
      condition = {
        type    = "Age"
        max_age = var.backup_retention_days * 86400
      }
    }

    # A failed multipart upload otherwise bills as storage forever.
    abort_multipart_uploads_transition = {
      condition = {
        type    = "Age"
        max_age = 86400
      }
    }
  }]
}

# ---------------------------------------------------------------------------
# Zone security.
#
# Declared rather than clicked, for the same reason the Grafana datasources are
# provisioned from files: a setting nobody can see is a setting nobody can
# review, and TLS settings are exactly the kind that get loosened once to debug
# something and never tightened again.
#
# dev.marum.loan and marum.loan live in ONE zone, so these settings are shared. Only
# the production state manages them; if both did, each apply would revert the
# other and the plan would never be empty.
# ---------------------------------------------------------------------------

resource "cloudflare_zone_setting" "always_use_https" {
  count = local.manages_zone ? 1 : 0

  zone_id    = var.cloudflare_zone_id
  setting_id = "always_use_https"
  value      = "on"
}

resource "cloudflare_zone_setting" "min_tls_version" {
  count = local.manages_zone ? 1 : 0

  zone_id    = var.cloudflare_zone_id
  setting_id = "min_tls_version"
  value      = "1.2"
}

resource "cloudflare_zone_setting" "tls_1_3" {
  count = local.manages_zone ? 1 : 0

  zone_id    = var.cloudflare_zone_id
  setting_id = "tls_1_3"
  value      = "on"
}

# Telegram signs webhooks and Marum verifies that signature itself, but a
# browser reaching the admin interface benefits from HSTS regardless.
resource "cloudflare_zone_setting" "security_header" {
  count = local.manages_zone ? 1 : 0

  zone_id    = var.cloudflare_zone_id
  setting_id = "security_header"

  value = {
    strict_transport_security = {
      enabled            = true
      max_age            = 31536000
      include_subdomains = true
      preload            = false
      nosniff            = true
    }
  }
}

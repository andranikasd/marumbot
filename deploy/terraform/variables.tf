# ---------------------------------------------------------------------------
# Credentials. Never given a default, never written to a .tfvars file.
# ---------------------------------------------------------------------------

variable "cloudflare_api_token" {
  description = "Cloudflare API token with Hyperdrive, DNS and Workers R2 edit rights."
  type        = string
  sensitive   = true
}

variable "cloudflare_account_id" {
  description = "Cloudflare account that owns the Worker, the Hyperdrive config and the buckets."
  type        = string
  sensitive   = true
}

variable "cloudflare_zone_id" {
  description = "Zone ID for the apex domain the bot is served from."
  type        = string
  sensitive   = true
}

# ---------------------------------------------------------------------------
# The database.
#
# Cloudflare does not sell a managed PostgreSQL. Its own database is D1, which
# is SQLite, and the design rejected it because there is no faithful local twin
# of it and the ledger's arithmetic must be identical everywhere. So Postgres is
# hosted elsewhere -- Neon by default -- and Hyperdrive pools and caches in
# front of it from the edge.
#
# Terraform does not create the database itself. There is no first-party Neon
# provider, and the database is the one resource in this stack whose accidental
# destruction cannot be undone by re-running apply. It is created once, by hand,
# and its connection details are passed in here.
# ---------------------------------------------------------------------------

variable "database" {
  description = "Origin PostgreSQL that Hyperdrive connects to."

  type = object({
    host     = string
    port     = optional(number, 5432)
    name     = string
    user     = string
    password = string
  })

  sensitive = true

  validation {
    condition     = can(regex("^[a-z0-9.-]+$", var.database.host)) && !can(regex("^(localhost|127\\.)", var.database.host))
    error_message = "database.host must be a public hostname; Hyperdrive dials it from Cloudflare's network, not from your machine."
  }
}

# ---------------------------------------------------------------------------
# Environment shape.
# ---------------------------------------------------------------------------

variable "environment" {
  description = "Which environment this state file describes."
  type        = string

  validation {
    condition     = contains(["dev", "production"], var.environment)
    error_message = "environment must be dev or production."
  }
}

variable "hostname" {
  description = "Fully qualified hostname the Worker answers on, e.g. dev.marum.am."
  type        = string
}

variable "origin_connection_limit" {
  description = <<-TEXT
    Server-side connections Hyperdrive may hold open to the origin. Keep this
    comfortably below the origin's own limit: Neon's free tier allows far fewer
    than a paid instance, and exhausting it fails migrations, not just queries.
  TEXT

  type    = number
  default = 5

  validation {
    condition     = var.origin_connection_limit >= 5 && var.origin_connection_limit <= 100
    error_message = "origin_connection_limit must be between 5 and 100; Hyperdrive rejects anything outside that."
  }
}

variable "query_cache_max_age" {
  description = <<-TEXT
    Seconds Hyperdrive may serve a cached result for a read query.

    Zero in production. Marum shows people what they owe, and a stale balance
    read back after a payment was recorded is a correctness bug that looks like
    a caching win. Only dev sets this above zero, and only to exercise the path.
  TEXT

  type    = number
  default = 0
}

variable "backup_retention_days" {
  description = "Days a database dump stays in R2 before lifecycle deletion."
  type        = number
  default     = 30
}

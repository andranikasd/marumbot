# The one value a deploy needs back.
#
# wrangler.toml carries a placeholder for the HYPERDRIVE binding until this is
# applied. Paste this ID in once per environment; it is stable across applies
# because nothing in the origin block forces replacement.

output "hyperdrive_id" {
  description = "Value for the HYPERDRIVE binding id in wrangler.toml."
  value       = cloudflare_hyperdrive_config.postgres.id
}

output "backup_bucket" {
  description = "R2 bucket that database dumps are written to."
  value       = cloudflare_r2_bucket.backups.name
}

output "wrangler_hint" {
  description = "Exactly what to change in deploy/cloudflare/wrangler.toml."

  value = <<-TEXT
    [[env.${var.environment}.hyperdrive]]
    binding = "HYPERDRIVE"
    id = "${cloudflare_hyperdrive_config.postgres.id}"
  TEXT
}

# Non-secret shape of production.

environment = "production"
hostname    = "marum.loan"

origin_connection_limit = 20

# Zero, and it stays zero. Marum's whole claim is that the number it shows is
# the number its inputs produce. A cached read served after a payment was
# recorded breaks that claim in the way least likely to be noticed.
query_cache_max_age = 0

backup_retention_days = 90

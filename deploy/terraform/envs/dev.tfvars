# Non-secret shape of the dev environment. Safe to commit -- every value here is
# public once the bot is reachable, and none of it grants access to anything.
#
# Credentials come from TF_VAR_* in the environment, never from this file.

environment = "dev"
hostname    = "dev.marum.loan"

# Dev shares a Neon free-tier project, which allows far fewer origin
# connections than production. Asking for more than the origin has fails
# migrations rather than queuing them.
origin_connection_limit = 5

# Non-zero only here, and only so the caching path is exercised somewhere
# before it is trusted. Five seconds is short enough that a stale balance
# cannot survive a conversation turn.
query_cache_max_age = 5

backup_retention_days = 7

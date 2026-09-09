resource "webhookr_project" "example" {
  name = "shop-events"
}

resource "webhookr_endpoint" "orders" {
  project_id = webhookr_project.example.id
  name       = "orders-webhook"
}

resource "webhookr_destination" "primary" {
  project_id  = webhookr_project.example.id
  endpoint_id = webhookr_endpoint.orders.id

  name = "orders-primary"
  url  = "https://example.com/hooks/orders"

  # Optional — values below are the provider defaults.
  method       = "POST"
  content_type = "application/json"
  is_enabled   = true

  # Timeout for a single delivery attempt. Omit it to use the platform
  # default (30s); each retry gets a fresh window rather than the remainder
  # of the previous one.
  timeout_ms = 30000

  headers = {
    "X-Source" = "webhookr"
  }
}

# A destination that keeps the platform retry policy: 5 retries, 10s initial
# interval, exponential backoff x2, capped at 5m, with jitter. Omitting the
# block is what keeps it following that default if the default ever changes.
resource "webhookr_destination" "default_retries" {
  project_id  = webhookr_project.example.id
  endpoint_id = webhookr_endpoint.orders.id

  name = "orders-default-retries"
  url  = "https://example.com/hooks/orders-default"
}

# A destination that pins its own policy. Every field is optional and an
# omitted field still inherits the platform default, so a destination that
# only wants fewer retries does not have to restate the whole curve.
resource "webhookr_destination" "custom_retries" {
  project_id  = webhookr_project.example.id
  endpoint_id = webhookr_endpoint.orders.id

  name       = "orders-slow-receiver"
  url        = "https://slow.example.com/hooks/orders"
  timeout_ms = 45000

  retry_policy = {
    max_retries         = 8
    initial_interval_ms = 5000
    backoff_strategy    = "exponential"
    backoff_multiplier  = 2
    max_interval_ms     = 900000
    jitter              = true
  }
}

# Fixed backoff waits the same amount every time. backoff_multiplier is
# rejected here rather than ignored, because a policy that silently drops a
# number you set is worse than one that refuses it.
resource "webhookr_destination" "fixed_retries" {
  project_id  = webhookr_project.example.id
  endpoint_id = webhookr_endpoint.orders.id

  name = "orders-fixed-retries"
  url  = "https://example.com/hooks/orders-fixed"

  retry_policy = {
    max_retries         = 3
    initial_interval_ms = 30000
    backoff_strategy    = "fixed"
  }
}

# What a delivery will actually do, with the platform defaults applied.
output "orders_effective_retry_policy" {
  value = webhookr_destination.custom_retries.effective_retry_policy
}

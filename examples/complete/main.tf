terraform {
  required_providers {
    webhookr = {
      source = "forgers-tech/webhookr"
    }
  }
}

provider "webhookr" {
  api_url   = var.api_url
  api_token = var.api_token
}

variable "api_url" {
  type        = string
  default     = "https://api.webhookr.tech"
  description = "Webhookr SVC base URL."
}

variable "api_token" {
  type        = string
  sensitive   = true
  description = "Webhookr API token (whk_...)."
}

# ---------------------------------------------------------------------------
# A project groups related endpoints.
# ---------------------------------------------------------------------------
resource "webhookr_project" "shop" {
  name = "shop-events"
}

# ---------------------------------------------------------------------------
# Endpoints receive incoming events for the project.
# ---------------------------------------------------------------------------
resource "webhookr_endpoint" "orders" {
  project_id = webhookr_project.shop.id
  name       = "orders-webhook"
}

resource "webhookr_endpoint" "payments" {
  project_id = webhookr_project.shop.id
  name       = "payments-webhook"
  is_active  = false
}

# ---------------------------------------------------------------------------
# Destinations fan an endpoint's events out to downstream HTTP targets.
# ---------------------------------------------------------------------------
resource "webhookr_destination" "orders_primary" {
  project_id  = webhookr_project.shop.id
  endpoint_id = webhookr_endpoint.orders.id

  name         = "orders-primary"
  url          = "https://example.com/hooks/orders"
  method       = "POST"
  content_type = "application/json"
  timeout_ms   = 15000

  headers = {
    "X-Source"      = "webhookr"
    "Authorization" = "Bearer ${var.downstream_token}"
  }
}

resource "webhookr_destination" "orders_backup" {
  project_id  = webhookr_project.shop.id
  endpoint_id = webhookr_endpoint.orders.id

  name       = "orders-backup"
  url        = "https://backup.example.com/hooks/orders"
  is_enabled = false
}

# A receiver known to be slow to recover: fewer, longer-spaced attempts than
# the platform default, and a longer window for each one.
resource "webhookr_destination" "orders_partner" {
  project_id  = webhookr_project.shop.id
  endpoint_id = webhookr_endpoint.orders.id

  name       = "orders-partner"
  url        = "https://partner.example.com/hooks/orders"
  timeout_ms = 45000

  retry_policy = {
    max_retries         = 8
    initial_interval_ms = 30000
    backoff_strategy    = "exponential"
    backoff_multiplier  = 2
    max_interval_ms     = 900000
    jitter              = true
  }
}

resource "webhookr_destination" "payments_primary" {
  project_id  = webhookr_project.shop.id
  endpoint_id = webhookr_endpoint.payments.id

  name   = "payments-primary"
  url    = "https://example.com/hooks/payments"
  method = "PUT"
}

variable "downstream_token" {
  type        = string
  sensitive   = true
  default     = "replace-me"
  description = "Bearer token forwarded to the downstream destination."
}

# ---------------------------------------------------------------------------
# Outputs
# ---------------------------------------------------------------------------
output "project_id" {
  value = webhookr_project.shop.id
}

output "endpoint_slugs" {
  value = {
    orders   = webhookr_endpoint.orders.slug
    payments = webhookr_endpoint.payments.slug
  }
}

output "destination_ids" {
  value = {
    orders_primary   = webhookr_destination.orders_primary.id
    orders_backup    = webhookr_destination.orders_backup.id
    payments_primary = webhookr_destination.payments_primary.id
  }
}

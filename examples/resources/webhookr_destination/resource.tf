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
  timeout_ms   = 30000
  is_enabled   = true

  headers = {
    "X-Source" = "webhookr"
  }
}

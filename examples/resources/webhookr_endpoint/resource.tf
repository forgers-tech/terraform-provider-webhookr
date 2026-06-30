resource "webhookr_project" "example" {
  name = "shop-events"
}

resource "webhookr_endpoint" "orders" {
  project_id = webhookr_project.example.id
  name       = "orders-webhook"
  is_active  = true # default
}

# The API generates a URL-safe slug (10-char nanoid) used to build the public
# ingest URL, e.g. https://api.webhookr.tech/v1/ingest/<slug>.
output "orders_slug" {
  value = webhookr_endpoint.orders.slug
}

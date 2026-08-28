resource "webhookr_project" "example" {
  name = "shop-events"
}

resource "webhookr_endpoint" "orders" {
  project_id = webhookr_project.example.id
  name       = "orders-webhook"
}

# Reject any request to this endpoint that is not signed with the secret below.
# Webhookr generates the secret when `secret` is left unset.
resource "webhookr_endpoint_hmac" "orders" {
  project_id  = webhookr_project.example.id
  endpoint_id = webhookr_endpoint.orders.id
}

# Bring your own secret, and read the signature from a custom header.
resource "webhookr_endpoint_hmac" "billing" {
  project_id  = webhookr_project.example.id
  endpoint_id = webhookr_endpoint.orders.id
  header_name = "X-Appsterisk-Signature"
  secret      = var.billing_signing_secret
}

variable "billing_signing_secret" {
  type      = string
  sensitive = true
}

# The secret is stored in Terraform state. Treat the state file as a secret:
# a remote backend with encryption at rest and restricted access.
output "orders_signing_secret" {
  value     = webhookr_endpoint_hmac.orders.secret
  sensitive = true
}

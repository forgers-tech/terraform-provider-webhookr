resource "webhookr_project" "example" {
  name = "shop-events"
}

resource "webhookr_endpoint" "stripe" {
  project_id = webhookr_project.example.id
  name       = "stripe-webhook"
}

# Verify inbound requests against Stripe's own signature scheme. The secret is
# the "Signing secret" shown on the webhook endpoint in the Stripe Dashboard —
# Webhookr never generates it, so it has to be supplied here.
#
# Note the attribute is `provider_name`, not `provider`: `provider` is a
# reserved meta-argument in every Terraform resource block.
resource "webhookr_endpoint_provider_verification" "stripe" {
  project_id    = webhookr_project.example.id
  endpoint_id   = webhookr_endpoint.stripe.id
  provider_name = "stripe"
  secret        = var.stripe_webhook_signing_secret
}

# Test mode and live mode are different Stripe webhook endpoints with different
# signing secrets. Use a separate Webhookr endpoint for each.
variable "stripe_webhook_signing_secret" {
  type      = string
  sensitive = true
}

# An endpoint has exactly one verification strategy. Declaring both this and
# `webhookr_endpoint_hmac` for the same endpoint is refused by the API with a
# 409 rather than one silently replacing the other.

# GitHub works the same way. Its scheme is HMAC-SHA256 over the raw body under
# `X-Hub-Signature-256`, so Webhookr needs no vendor SDK for it — only the
# secret you set on the repository or organisation webhook.
resource "webhookr_endpoint" "github" {
  project_id = webhookr_project.example.id
  name       = "github-webhook"
}

resource "webhookr_endpoint_provider_verification" "github" {
  project_id    = webhookr_project.example.id
  endpoint_id   = webhookr_endpoint.github.id
  provider_name = "github"
  secret        = var.github_webhook_secret
}

variable "github_webhook_secret" {
  type      = string
  sensitive = true
}

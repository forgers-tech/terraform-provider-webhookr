terraform {
  required_providers {
    webhookr = {
      source = "forgers-tech/webhookr"
    }
  }
}

# --- API token authentication (recommended) ---------------------------------
# api_url and api_token may also be supplied via the WEBHOOKR_API_URL and
# WEBHOOKR_API_TOKEN environment variables instead of the provider block.
provider "webhookr" {
  api_url   = "https://api.webhookr.tech"
  api_token = var.webhookr_api_token # whk_...
}

variable "webhookr_api_token" {
  type        = string
  sensitive   = true
  description = "Webhookr API token (whk_...)."
}

# --- Firebase service-account authentication (alternative) ------------------
# Mutually exclusive with api_token. Useful for CI that already holds a
# Firebase service account. Values can also come from the matching
# WEBHOOKR_FIREBASE_API_KEY / WEBHOOKR_SERVICE_ACCOUNT_EMAIL /
# WEBHOOKR_SERVICE_ACCOUNT_KEY environment variables.
#
# provider "webhookr" {
#   api_url               = "https://api.webhookr.tech"
#   firebase_api_key      = var.firebase_api_key
#   service_account_email = var.service_account_email
#   service_account_key   = var.service_account_key # RSA private key, PEM
# }

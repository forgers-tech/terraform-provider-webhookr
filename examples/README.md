# Examples

This directory holds runnable examples for the `forgers-tech/webhookr` provider.
The layout follows the conventions used by
[`terraform-plugin-docs`](https://github.com/hashicorp/terraform-plugin-docs),
so these files double as the source for generated documentation.

| Path | What it shows |
| --- | --- |
| `provider/` | Provider configuration — API-token auth and the Firebase service-account alternative. |
| `resources/webhookr_project/` | Minimal `webhookr_project`. |
| `resources/webhookr_endpoint/` | An endpoint and its generated `slug`. |
| `resources/webhookr_destination/` | A destination with headers and delivery options. |
| `complete/` | A full project → endpoints → destinations topology with outputs. |

## Running an example

```sh
cd complete

export WEBHOOKR_API_TOKEN="whk_..."        # or pass -var 'api_token=...'
export TF_VAR_api_token="$WEBHOOKR_API_TOKEN"

terraform init
terraform apply
```

Set `api_url` (or `WEBHOOKR_API_URL`) to your Webhookr SVC base URL. Tear the
example down again with `terraform destroy` — deleting the project cascades to
its endpoints and destinations.

### Local development

When testing against a locally built provider, skip `terraform init` and use a
[`dev_overrides`](https://developer.hashicorp.com/terraform/cli/config/config-file#development-overrides-for-provider-developers)
block in a CLI config file pointing at your `go install` output, then run
`terraform plan` / `apply` directly.

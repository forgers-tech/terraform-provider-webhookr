package provider

import (
	"context"
	"os"

	"github.com/forgers-tech/terraform-provider-webhookr/internal/auth"
	"github.com/forgers-tech/terraform-provider-webhookr/internal/client"
	tfresource "github.com/forgers-tech/terraform-provider-webhookr/internal/resource"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ provider.Provider = (*WebhookrProvider)(nil)

// WebhookrProvider implements the Terraform provider for Webhookr.
type WebhookrProvider struct {
	version string
}

// WebhookrProviderModel maps the provider schema to Go types.
type WebhookrProviderModel struct {
	APIURL              types.String `tfsdk:"api_url"`
	FirebaseAPIKey      types.String `tfsdk:"firebase_api_key"`
	ServiceAccountEmail types.String `tfsdk:"service_account_email"`
	ServiceAccountKey   types.String `tfsdk:"service_account_key"`
}

// New returns the provider factory used by providerserver.Serve.
func New(version string) func() provider.Provider {
	return func() provider.Provider {
		return &WebhookrProvider{version: version}
	}
}

func (p *WebhookrProvider) Metadata(_ context.Context, _ provider.MetadataRequest, resp *provider.MetadataResponse) {
	resp.TypeName = "webhookr"
	resp.Version = p.version
}

func (p *WebhookrProvider) Schema(_ context.Context, _ provider.SchemaRequest, resp *provider.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manage Webhookr resources — projects, endpoints, and destinations — " +
			"authenticated via a Firebase service account.",
		Attributes: map[string]schema.Attribute{
			"api_url": schema.StringAttribute{
				Required:    true,
				Description: "Webhookr SVC base URL (e.g. https://api.webhookr.tech). Env: WEBHOOKR_API_URL.",
			},
			"firebase_api_key": schema.StringAttribute{
				Required:    true,
				Sensitive:   true,
				Description: "Firebase Web API key — used to exchange custom tokens for ID tokens. Env: WEBHOOKR_FIREBASE_API_KEY.",
			},
			"service_account_email": schema.StringAttribute{
				Required:    true,
				Description: "Firebase service account email (name@project.iam.gserviceaccount.com). Env: WEBHOOKR_SERVICE_ACCOUNT_EMAIL.",
			},
			"service_account_key": schema.StringAttribute{
				Required:    true,
				Sensitive:   true,
				Description: "Firebase service account RSA private key in PEM format. Env: WEBHOOKR_SERVICE_ACCOUNT_KEY.",
			},
		},
	}
}

func (p *WebhookrProvider) Configure(ctx context.Context, req provider.ConfigureRequest, resp *provider.ConfigureResponse) {
	var cfg WebhookrProviderModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}

	apiURL := resolveString(cfg.APIURL, "WEBHOOKR_API_URL")
	firebaseAPIKey := resolveString(cfg.FirebaseAPIKey, "WEBHOOKR_FIREBASE_API_KEY")
	serviceAccountEmail := resolveString(cfg.ServiceAccountEmail, "WEBHOOKR_SERVICE_ACCOUNT_EMAIL")
	serviceAccountKey := resolveString(cfg.ServiceAccountKey, "WEBHOOKR_SERVICE_ACCOUNT_KEY")

	if apiURL == "" {
		resp.Diagnostics.AddError("Missing api_url",
			"Set api_url in the provider block or WEBHOOKR_API_URL environment variable.")
	}
	if firebaseAPIKey == "" {
		resp.Diagnostics.AddError("Missing firebase_api_key",
			"Set firebase_api_key in the provider block or WEBHOOKR_FIREBASE_API_KEY environment variable.")
	}
	if serviceAccountEmail == "" {
		resp.Diagnostics.AddError("Missing service_account_email",
			"Set service_account_email in the provider block or WEBHOOKR_SERVICE_ACCOUNT_EMAIL environment variable.")
	}
	if serviceAccountKey == "" {
		resp.Diagnostics.AddError("Missing service_account_key",
			"Set service_account_key in the provider block or WEBHOOKR_SERVICE_ACCOUNT_KEY environment variable.")
	}
	if resp.Diagnostics.HasError() {
		return
	}

	firebaseClient, err := auth.New(firebaseAPIKey, serviceAccountEmail, serviceAccountKey)
	if err != nil {
		resp.Diagnostics.AddError("Invalid service_account_key", err.Error())
		return
	}

	apiClient := client.New(apiURL, firebaseClient)
	resp.DataSourceData = apiClient
	resp.ResourceData = apiClient
}

func (p *WebhookrProvider) Resources(_ context.Context) []func() resource.Resource {
	return []func() resource.Resource{
		tfresource.NewProjectResource,
		tfresource.NewEndpointResource,
		tfresource.NewDestinationResource,
	}
}

func (p *WebhookrProvider) DataSources(_ context.Context) []func() datasource.DataSource {
	return []func() datasource.DataSource{}
}

// resolveString returns the Terraform config value when set, falling back to
// the named environment variable.
func resolveString(attr types.String, envVar string) string {
	if !attr.IsNull() && !attr.IsUnknown() {
		return attr.ValueString()
	}
	return os.Getenv(envVar)
}

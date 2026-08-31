package resource

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/forgers-tech/terraform-provider-webhookr/internal/client"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var (
	_ resource.Resource                   = (*EndpointProviderVerificationResource)(nil)
	_ resource.ResourceWithValidateConfig = (*EndpointProviderVerificationResource)(nil)
)

// Providers whose signature scheme the Webhookr ingest edge can verify. Adding
// one here without the matching verifier in webhookr-ingest would produce an
// endpoint that rejects every request as unverifiable.
var supportedVerificationProviders = []string{"stripe", "github"}

type EndpointProviderVerificationResource struct {
	client *client.Client
}

type endpointProviderVerificationModel struct {
	ProjectID    types.String `tfsdk:"project_id"`
	EndpointID   types.String `tfsdk:"endpoint_id"`
	ProviderName types.String `tfsdk:"provider_name"`
	Secret       types.String `tfsdk:"secret"`
}

// Response of PUT and DELETE on the provider verification sub-resource. It
// carries no secret — not even the one just sent.
type endpointProviderVerificationAPIResponse struct {
	Type     string `json:"type"`
	Provider string `json:"provider"`
}

type endpointVerificationReadResponse struct {
	Verification struct {
		Type     string `json:"type"`
		Provider string `json:"provider"`
	} `json:"verification"`
}

func NewEndpointProviderVerificationResource() resource.Resource {
	return &EndpointProviderVerificationResource{}
}

func (r *EndpointProviderVerificationResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_endpoint_provider_verification"
}

func (r *EndpointProviderVerificationResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Verifies inbound requests to a Webhookr endpoint against a provider's own " +
			"signature scheme — Stripe and GitHub today. Requests that do not verify are rejected " +
			"at the ingest edge and never enqueued.\n\n" +
			"An endpoint has exactly one verification strategy. Configuring this on an endpoint " +
			"that already uses `webhookr_endpoint_hmac` is refused by the API; destroy that " +
			"resource first.\n\n" +
			"The signing secret is written to Terraform state, because Terraform has to send it " +
			"to Webhookr. Unlike `webhookr_endpoint_hmac`, Webhookr never generates or returns " +
			"this secret — it comes from the provider — so the state file is the only place " +
			"Terraform can keep it. Treat the state file as a secret: use a remote backend with " +
			"encryption at rest and restricted access, and keep the value in a variable rather " +
			"than inline.",
		Attributes: map[string]schema.Attribute{
			"project_id": schema.StringAttribute{
				Required:    true,
				Description: "ID of the parent project.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"endpoint_id": schema.StringAttribute{
				Required:    true,
				Description: "ID of the endpoint to protect.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			// Not `provider`: that is a reserved meta-argument in every
			// Terraform resource block and would be consumed by Terraform core
			// before it ever reached this schema.
			"provider_name": schema.StringAttribute{
				Required: true,
				Description: "Provider whose signature scheme this endpoint is verified against. " +
					"Supported values: " + strings.Join(supportedVerificationProviders, ", ") + ".",
			},
			"secret": schema.StringAttribute{
				Required:  true,
				Sensitive: true,
				Description: "The provider's webhook signing secret — for Stripe, the `whsec_…` " +
					"value shown on the webhook endpoint in the Stripe Dashboard; for GitHub, the " +
					"secret set on the repository or organisation webhook. Webhookr never returns " +
					"it, so Terraform cannot detect a change made outside Terraform; changing it " +
					"here re-keys the endpoint immediately.",
			},
		},
	}
}

func (r *EndpointProviderVerificationResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	c, ok := req.ProviderData.(*client.Client)
	if !ok {
		resp.Diagnostics.AddError("Unexpected provider data type",
			fmt.Sprintf("expected *client.Client, got %T", req.ProviderData))
		return
	}
	r.client = c
}

// Rejects an unsupported provider at plan time rather than letting the API
// answer 400 during apply, when half the plan may already have been applied.
func (r *EndpointProviderVerificationResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var config endpointProviderVerificationModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if config.ProviderName.IsUnknown() || config.ProviderName.IsNull() {
		return
	}

	name := config.ProviderName.ValueString()
	for _, supported := range supportedVerificationProviders {
		if name == supported {
			return
		}
	}
	resp.Diagnostics.AddAttributeError(
		path.Root("provider_name"),
		"Unsupported verification provider",
		fmt.Sprintf("%q is not a provider Webhookr can verify. Supported values: %s.",
			name, strings.Join(supportedVerificationProviders, ", ")),
	)
}

func (r *EndpointProviderVerificationResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan endpointProviderVerificationModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if !r.configure(ctx, plan, &resp.Diagnostics) {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *EndpointProviderVerificationResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state endpointProviderVerificationModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	endpointURL := endpointPath(state.ProjectID.ValueString(), state.EndpointID.ValueString())
	var result endpointVerificationReadResponse
	// client.Do returns both a status and an error for anything >= 400, so the
	// status is consulted first: a 404 is the endpoint being gone, not a fault.
	status, err := r.client.Do(ctx, http.MethodGet, endpointURL, nil, &result)
	if status == http.StatusNotFound {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Error reading endpoint verification configuration", err.Error())
		return
	}
	if status != http.StatusOK {
		resp.Diagnostics.AddError("Unexpected status reading endpoint verification configuration",
			fmt.Sprintf("expected 200, got %d", status))
		return
	}
	// Turned off, or switched to the generic HMAC scheme, outside Terraform:
	// this resource no longer describes anything.
	if result.Verification.Type != "provider" {
		resp.State.RemoveResource(ctx)
		return
	}

	// The API never returns the secret, so state keeps the value it was created
	// with. A secret replaced outside Terraform is therefore invisible here —
	// documented on the attribute rather than papered over.
	state.ProviderName = types.StringValue(result.Verification.Provider)
	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

func (r *EndpointProviderVerificationResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan endpointProviderVerificationModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Unlike the HMAC resource there is no "re-send the stored secret" case:
	// the secret is required, so the plan always carries the value the
	// configuration means, and a PUT with it is the whole update.
	if !r.configure(ctx, plan, &resp.Diagnostics) {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *EndpointProviderVerificationResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state endpointProviderVerificationModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	deleteURL := endpointProviderVerificationPath(state.ProjectID.ValueString(), state.EndpointID.ValueString())
	status, err := r.client.Do(ctx, http.MethodDelete, deleteURL, nil, nil)
	if status == http.StatusNotFound || status == http.StatusOK {
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Error disabling provider verification", err.Error())
		return
	}
	resp.Diagnostics.AddError("Unexpected status disabling provider verification",
		fmt.Sprintf("expected 200, got %d", status))
}

func (r *EndpointProviderVerificationResource) configure(
	ctx context.Context,
	plan endpointProviderVerificationModel,
	diags *diag.Diagnostics,
) bool {
	body := map[string]any{
		"provider": plan.ProviderName.ValueString(),
		"secret":   plan.Secret.ValueString(),
	}

	configureURL := endpointProviderVerificationPath(plan.ProjectID.ValueString(), plan.EndpointID.ValueString())
	var result endpointProviderVerificationAPIResponse
	status, err := r.client.Do(ctx, http.MethodPut, configureURL, body, &result)
	if status == http.StatusConflict {
		diags.AddError("Endpoint already verifies a generic HMAC signature",
			"An endpoint has exactly one verification strategy. Destroy the "+
				"webhookr_endpoint_hmac resource for this endpoint before configuring a "+
				"provider, so its signing secret is retired deliberately rather than as a "+
				"side effect.")
		return false
	}
	if err != nil {
		diags.AddError("Error configuring provider verification", err.Error())
		return false
	}
	if status != http.StatusOK {
		diags.AddError("Unexpected status configuring provider verification",
			fmt.Sprintf("expected 200, got %d", status))
		return false
	}
	return true
}

func endpointProviderVerificationPath(projectID, endpointID string) string {
	return endpointPath(projectID, endpointID) + "/verification/provider"
}

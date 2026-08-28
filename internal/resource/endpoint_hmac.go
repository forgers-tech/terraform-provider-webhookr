package resource

import (
	"context"
	"fmt"
	"net/http"

	"github.com/forgers-tech/terraform-provider-webhookr/internal/client"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ resource.Resource = (*EndpointHmacResource)(nil)

type EndpointHmacResource struct {
	client *client.Client
}

type endpointHmacModel struct {
	ProjectID  types.String `tfsdk:"project_id"`
	EndpointID types.String `tfsdk:"endpoint_id"`
	HeaderName types.String `tfsdk:"header_name"`
	Secret     types.String `tfsdk:"secret"`
	Algorithm  types.String `tfsdk:"algorithm"`
	KeyID      types.String `tfsdk:"key_id"`
}

// Response of PUT /hmac. `secret` is present only here; every other endpoint of
// the API omits it.
type endpointHmacAPIResponse struct {
	Enabled    bool   `json:"enabled"`
	Algorithm  string `json:"algorithm"`
	HeaderName string `json:"headerName"`
	KeyID      string `json:"keyId"`
	Secret     string `json:"secret"`
}

type endpointHmacReadResponse struct {
	Hmac struct {
		Enabled    bool   `json:"enabled"`
		Algorithm  string `json:"algorithm"`
		HeaderName string `json:"headerName"`
		KeyID      string `json:"keyId"`
	} `json:"hmac"`
}

func NewEndpointHmacResource() resource.Resource {
	return &EndpointHmacResource{}
}

func (r *EndpointHmacResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_endpoint_hmac"
}

func (r *EndpointHmacResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Enables HMAC-SHA256 signature verification on a Webhookr endpoint. " +
			"Requests that do not carry a valid signature are rejected at the ingest edge " +
			"and never enqueued.\n\n" +
			"The signing secret is written to Terraform state, because Terraform must be " +
			"able to hand it to whatever configures the sending system. Treat the state " +
			"file as a secret: use a remote backend with encryption at rest and restricted " +
			"access.",
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
			"header_name": schema.StringAttribute{
				Optional:    true,
				Computed:    true,
				Description: "Request header the signature is read from. Defaults to X-Webhookr-Signature.",
			},
			"secret": schema.StringAttribute{
				Optional:  true,
				Computed:  true,
				Sensitive: true,
				Description: "The signing secret. Leave unset to have Webhookr generate one; " +
					"set it to bring your own (16-256 printable ASCII characters). " +
					"Changing it re-keys the endpoint immediately.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"algorithm": schema.StringAttribute{
				Computed:    true,
				Description: "Signature algorithm. Currently always sha256.",
			},
			"key_id": schema.StringAttribute{
				Computed:    true,
				Description: "Identifier of the active signing key, for correlating rotations.",
			},
		},
	}
}

func (r *EndpointHmacResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *EndpointHmacResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan endpointHmacModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	result, ok := r.configure(ctx, plan, plan.Secret, &resp.Diagnostics)
	if !ok {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, hmacAPIToModel(plan, result))...)
}

func (r *EndpointHmacResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state endpointHmacModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	path := endpointPath(state.ProjectID.ValueString(), state.EndpointID.ValueString())
	var result endpointHmacReadResponse
	status, err := r.client.Do(ctx, http.MethodGet, path, nil, &result)
	if status == http.StatusNotFound {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Error reading endpoint HMAC configuration", err.Error())
		return
	}
	if status != http.StatusOK {
		resp.Diagnostics.AddError("Unexpected status reading endpoint HMAC configuration",
			fmt.Sprintf("expected 200, got %d", status))
		return
	}
	// Verification turned off outside Terraform: the resource no longer exists.
	if !result.Hmac.Enabled {
		resp.State.RemoveResource(ctx)
		return
	}

	// The API never returns the secret again, so state keeps the value it was
	// created with. Everything else is refreshed from the API.
	state.Algorithm = types.StringValue(result.Hmac.Algorithm)
	state.HeaderName = types.StringValue(result.Hmac.HeaderName)
	state.KeyID = types.StringValue(result.Hmac.KeyID)
	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

func (r *EndpointHmacResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state endpointHmacModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Re-send the existing secret unless the configuration asks for a new one.
	// The API generates a fresh secret whenever none is supplied, which would
	// silently re-key the endpoint on an unrelated change such as a renamed
	// header and break every sender still using the old one.
	secret := plan.Secret
	if secret.IsUnknown() || secret.IsNull() {
		secret = state.Secret
	}

	result, ok := r.configure(ctx, plan, secret, &resp.Diagnostics)
	if !ok {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, hmacAPIToModel(plan, result))...)
}

func (r *EndpointHmacResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state endpointHmacModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	path := endpointHmacPath(state.ProjectID.ValueString(), state.EndpointID.ValueString())
	status, err := r.client.Do(ctx, http.MethodDelete, path, nil, nil)
	if status == http.StatusNotFound || status == http.StatusOK {
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Error disabling endpoint HMAC verification", err.Error())
		return
	}
	resp.Diagnostics.AddError("Unexpected status disabling endpoint HMAC verification",
		fmt.Sprintf("expected 200, got %d", status))
}

func (r *EndpointHmacResource) configure(
	ctx context.Context,
	plan endpointHmacModel,
	secret types.String,
	diags *diag.Diagnostics,
) (endpointHmacAPIResponse, bool) {
	body := map[string]any{}
	if !plan.HeaderName.IsUnknown() && !plan.HeaderName.IsNull() {
		body["headerName"] = plan.HeaderName.ValueString()
	}
	if !secret.IsUnknown() && !secret.IsNull() {
		body["secret"] = secret.ValueString()
	}

	path := endpointHmacPath(plan.ProjectID.ValueString(), plan.EndpointID.ValueString())
	var result endpointHmacAPIResponse
	status, err := r.client.Do(ctx, http.MethodPut, path, body, &result)
	if err != nil {
		diags.AddError("Error configuring endpoint HMAC verification", err.Error())
		return result, false
	}
	if status != http.StatusOK {
		diags.AddError("Unexpected status configuring endpoint HMAC verification",
			fmt.Sprintf("expected 200, got %d", status))
		return result, false
	}
	return result, true
}

func endpointHmacPath(projectID, endpointID string) string {
	return endpointPath(projectID, endpointID) + "/hmac"
}

func hmacAPIToModel(plan endpointHmacModel, result endpointHmacAPIResponse) endpointHmacModel {
	return endpointHmacModel{
		ProjectID:  plan.ProjectID,
		EndpointID: plan.EndpointID,
		HeaderName: types.StringValue(result.HeaderName),
		Secret:     types.StringValue(result.Secret),
		Algorithm:  types.StringValue(result.Algorithm),
		KeyID:      types.StringValue(result.KeyID),
	}
}

package resource

import (
	"context"
	"fmt"
	"net/http"

	"github.com/forgers-tech/terraform-provider-webhookr/internal/client"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ resource.Resource = (*DestinationResource)(nil)

type DestinationResource struct {
	client *client.Client
}

type destinationModel struct {
	ID                   types.String `tfsdk:"id"`
	ProjectID            types.String `tfsdk:"project_id"`
	EndpointID           types.String `tfsdk:"endpoint_id"`
	Name                 types.String `tfsdk:"name"`
	URL                  types.String `tfsdk:"url"`
	Method               types.String `tfsdk:"method"`
	Headers              types.Map    `tfsdk:"headers"`
	ContentType          types.String `tfsdk:"content_type"`
	TimeoutMs            types.Int64  `tfsdk:"timeout_ms"`
	RetryPolicy          *retryPolicy `tfsdk:"retry_policy"`
	EffectiveRetryPolicy types.Object `tfsdk:"effective_retry_policy"`
	IsEnabled            types.Bool   `tfsdk:"is_enabled"`
	CreatedAt            types.String `tfsdk:"created_at"`
	UpdatedAt            types.String `tfsdk:"updated_at"`
}

// A destination's own retry overrides. Every field is optional and an omitted
// field inherits the documented platform default, so a destination that only
// wants fewer retries does not have to restate the whole backoff curve.
type retryPolicy struct {
	MaxRetries        types.Int64   `tfsdk:"max_retries"`
	InitialIntervalMs types.Int64   `tfsdk:"initial_interval_ms"`
	BackoffStrategy   types.String  `tfsdk:"backoff_strategy"`
	BackoffMultiplier types.Float64 `tfsdk:"backoff_multiplier"`
	MaxIntervalMs     types.Int64   `tfsdk:"max_interval_ms"`
	Jitter            types.Bool    `tfsdk:"jitter"`
}

// The overrides as the API returns them. Pointers throughout because null is
// meaningful here: it says "this field follows the platform default", which is
// a different fact from the number that default happens to be today.
type retryPolicyAPI struct {
	MaxRetries        *int64   `json:"maxRetries"`
	InitialIntervalMs *int64   `json:"initialIntervalMs"`
	BackoffStrategy   *string  `json:"backoffStrategy"`
	BackoffMultiplier *float64 `json:"backoffMultiplier"`
	MaxIntervalMs     *int64   `json:"maxIntervalMs"`
	Jitter            *bool    `json:"jitter"`
}

// The policy after defaults are applied — what a delivery will actually do.
// Nothing is a pointer: that is the point of it.
type effectiveRetryPolicyAPI struct {
	MaxRetries        int64   `json:"maxRetries"`
	InitialIntervalMs int64   `json:"initialIntervalMs"`
	BackoffStrategy   string  `json:"backoffStrategy"`
	BackoffMultiplier float64 `json:"backoffMultiplier"`
	MaxIntervalMs     int64   `json:"maxIntervalMs"`
	Jitter            bool    `json:"jitter"`
	TimeoutMs         int64   `json:"timeoutMs"`
}

type destinationAPIResponse struct {
	ID          string            `json:"id"`
	EndpointID  string            `json:"endpointId"`
	Name        string            `json:"name"`
	URL         string            `json:"url"`
	Method      string            `json:"method"`
	Headers     map[string]string `json:"headers"`
	ContentType string            `json:"contentType"`
	// Nullable since the service made a destination timeout optional; null
	// means the destination follows the platform default.
	TimeoutMs            *int64                  `json:"timeoutMs"`
	RetryPolicy          retryPolicyAPI          `json:"retryPolicy"`
	EffectiveRetryPolicy effectiveRetryPolicyAPI `json:"effectiveRetryPolicy"`
	IsEnabled            bool                    `json:"isEnabled"`
	CreatedAt            string                  `json:"createdAt"`
	UpdatedAt            string                  `json:"updatedAt"`
}

var effectiveRetryPolicyAttrTypes = map[string]attr.Type{
	"max_retries":         types.Int64Type,
	"initial_interval_ms": types.Int64Type,
	"backoff_strategy":    types.StringType,
	"backoff_multiplier":  types.Float64Type,
	"max_interval_ms":     types.Int64Type,
	"jitter":              types.BoolType,
	"timeout_ms":          types.Int64Type,
}

func NewDestinationResource() resource.Resource {
	return &DestinationResource{}
}

func (r *DestinationResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_destination"
}

func (r *DestinationResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a Webhookr destination (webhook delivery target) for an endpoint.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:    true,
				Description: "Unique identifier of the destination.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"project_id": schema.StringAttribute{
				Required:    true,
				Description: "ID of the parent project.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"endpoint_id": schema.StringAttribute{
				Required:    true,
				Description: "ID of the parent endpoint.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"name": schema.StringAttribute{
				Required:    true,
				Description: "Display name of the destination (max 100 characters).",
			},
			"url": schema.StringAttribute{
				Required:    true,
				Description: "HTTPS URL where webhook events are delivered.",
			},
			"method": schema.StringAttribute{
				Optional:    true,
				Computed:    true,
				Default:     stringdefault.StaticString("POST"),
				Description: "HTTP method used for delivery. One of: GET, POST, PUT, PATCH, DELETE.",
			},
			"headers": schema.MapAttribute{
				Optional:    true,
				Computed:    true,
				ElementType: types.StringType,
				Description: "Custom HTTP headers sent with every delivery (max 20 entries).",
			},
			"content_type": schema.StringAttribute{
				Optional:    true,
				Computed:    true,
				Default:     stringdefault.StaticString("application/json"),
				Description: "Content-Type header for the delivery request.",
			},
			// No default. The service made a destination timeout optional, and
			// pinning 30000 here would mean every Terraform-managed destination
			// stops following the platform default the moment it changes.
			// Existing state is unaffected: destinations created before the
			// timeout became optional kept their explicit 30000, so an
			// unconfigured destination still reads back 30000 and plans clean.
			"timeout_ms": schema.Int64Attribute{
				Optional:    true,
				Computed:    true,
				Description: "Timeout for a single delivery attempt, in milliseconds (1000–60000). Omit to use the platform default (30s). Each retry gets a fresh window.",
			},
			"retry_policy": schema.SingleNestedAttribute{
				Optional:    true,
				Description: "Per-destination retry policy. Omit to use the documented platform default: 5 retries, 10s initial interval, exponential backoff with a multiplier of 2, capped at 5m, with jitter.",
				Attributes: map[string]schema.Attribute{
					"max_retries": schema.Int64Attribute{
						Optional:    true,
						Description: "Retries after the original delivery, so N allows N+1 attempts in total (0–20). 0 disables retries.",
					},
					"initial_interval_ms": schema.Int64Attribute{
						Optional:    true,
						Description: "Delay before the first retry, in milliseconds (1000–3600000). Must not exceed max_interval_ms.",
					},
					"backoff_strategy": schema.StringAttribute{
						Optional:    true,
						Description: "One of: fixed, exponential.",
					},
					"backoff_multiplier": schema.Float64Attribute{
						Optional:    true,
						Description: "Growth rate for exponential backoff (1–10). Rejected by the API under fixed backoff, where it would have no meaning.",
					},
					"max_interval_ms": schema.Int64Attribute{
						Optional:    true,
						Description: "Ceiling on any single retry delay, in milliseconds (1000–86400000). Applies after jitter and after Retry-After.",
					},
					"jitter": schema.BoolAttribute{
						Optional:    true,
						Description: "Spread each delay uniformly over the upper half of its window, so deliveries that fail together do not retry together.",
					},
				},
			},
			// The overrides above say what this destination pins; this says what
			// a delivery will actually do. Exposed because the merged result is
			// what an operator needs to see in a plan, and it is the same policy
			// the platform records against every attempt.
			"effective_retry_policy": schema.SingleNestedAttribute{
				Computed:    true,
				Description: "The retry policy in force, with platform defaults applied.",
				Attributes: map[string]schema.Attribute{
					"max_retries":         schema.Int64Attribute{Computed: true},
					"initial_interval_ms": schema.Int64Attribute{Computed: true},
					"backoff_strategy":    schema.StringAttribute{Computed: true},
					"backoff_multiplier":  schema.Float64Attribute{Computed: true},
					"max_interval_ms":     schema.Int64Attribute{Computed: true},
					"jitter":              schema.BoolAttribute{Computed: true},
					"timeout_ms":          schema.Int64Attribute{Computed: true},
				},
			},
			"is_enabled": schema.BoolAttribute{
				Optional:    true,
				Computed:    true,
				Default:     booldefault.StaticBool(true),
				Description: "Whether this destination is active and receives event deliveries.",
			},
			"created_at": schema.StringAttribute{
				Computed:    true,
				Description: "RFC3339 timestamp of when the destination was created.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"updated_at": schema.StringAttribute{
				Computed:    true,
				Description: "RFC3339 timestamp of the last destination update.",
			},
		},
	}
}

func (r *DestinationResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *DestinationResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan destinationModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	body, bodyDiags := destinationModelToBody(ctx, plan)
	resp.Diagnostics.Append(bodyDiags...)
	if resp.Diagnostics.HasError() {
		return
	}

	path := destinationPath(plan.ProjectID.ValueString(), plan.EndpointID.ValueString(), "")
	var result destinationAPIResponse
	status, err := r.client.Do(ctx, http.MethodPost, path, body, &result)
	if err != nil {
		resp.Diagnostics.AddError("Error creating destination", err.Error())
		return
	}
	if status != http.StatusCreated {
		resp.Diagnostics.AddError("Unexpected status creating destination",
			fmt.Sprintf("expected 201, got %d", status))
		return
	}

	model, diags := destinationAPIToModel(ctx, result, plan.ProjectID.ValueString())
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, model)...)
}

func (r *DestinationResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state destinationModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	path := destinationPath(state.ProjectID.ValueString(), state.EndpointID.ValueString(), state.ID.ValueString())
	var result destinationAPIResponse
	status, err := r.client.Do(ctx, http.MethodGet, path, nil, &result)
	if status == http.StatusNotFound {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Error reading destination", err.Error())
		return
	}
	if status != http.StatusOK {
		resp.Diagnostics.AddError("Unexpected status reading destination",
			fmt.Sprintf("expected 200, got %d", status))
		return
	}

	model, diags := destinationAPIToModel(ctx, result, state.ProjectID.ValueString())
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, model)...)
}

func (r *DestinationResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan destinationModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var state destinationModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	body, bodyDiags := destinationModelToBody(ctx, plan)
	resp.Diagnostics.Append(bodyDiags...)
	if resp.Diagnostics.HasError() {
		return
	}

	path := destinationPath(state.ProjectID.ValueString(), state.EndpointID.ValueString(), state.ID.ValueString())
	var result destinationAPIResponse
	status, err := r.client.Do(ctx, http.MethodPatch, path, body, &result)
	if err != nil {
		resp.Diagnostics.AddError("Error updating destination", err.Error())
		return
	}
	if status != http.StatusOK {
		resp.Diagnostics.AddError("Unexpected status updating destination",
			fmt.Sprintf("expected 200, got %d", status))
		return
	}

	model, diags := destinationAPIToModel(ctx, result, state.ProjectID.ValueString())
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, model)...)
}

func (r *DestinationResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state destinationModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	path := destinationPath(state.ProjectID.ValueString(), state.EndpointID.ValueString(), state.ID.ValueString())
	status, err := r.client.Do(ctx, http.MethodDelete, path, nil, nil)
	if status == http.StatusNotFound {
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Error deleting destination", err.Error())
		return
	}
	if status != http.StatusOK {
		resp.Diagnostics.AddError("Unexpected status deleting destination",
			fmt.Sprintf("expected 200, got %d", status))
		return
	}
}

func destinationPath(projectID, endpointID, destinationID string) string {
	base := "/v1/projects/" + projectID + "/endpoints/" + endpointID + "/destinations"
	if destinationID == "" {
		return base
	}
	return base + "/" + destinationID
}

func destinationModelToBody(ctx context.Context, m destinationModel) (map[string]any, diag.Diagnostics) {
	body := map[string]any{
		"name":        m.Name.ValueString(),
		"url":         m.URL.ValueString(),
		"method":      m.Method.ValueString(),
		"contentType": m.ContentType.ValueString(),
		"isEnabled":   m.IsEnabled.ValueBool(),
		// Explicit null rather than omitted. Terraform configuration is the
		// source of truth, so a policy removed from the config has to be
		// cleared on the destination; omitting the key would leave whatever
		// was stored in place and the resource would never converge.
		"retryPolicy": retryPolicyToBody(m.RetryPolicy),
	}

	// Unknown means Terraform will fill it from the response; sending it would
	// serialise as 0, which the API rejects as below the minimum.
	if m.TimeoutMs.IsUnknown() {
		body["timeoutMs"] = nil
	} else if m.TimeoutMs.IsNull() {
		body["timeoutMs"] = nil
	} else {
		body["timeoutMs"] = m.TimeoutMs.ValueInt64()
	}

	if !m.Headers.IsNull() && !m.Headers.IsUnknown() {
		headers := make(map[string]string, len(m.Headers.Elements()))
		diags := m.Headers.ElementsAs(ctx, &headers, false)
		if diags.HasError() {
			return nil, diags
		}
		body["headers"] = headers
	}
	return body, nil
}

// The API's write shape drops the `Ms` suffix on the two durations, because it
// also accepts unit strings such as "10s"; the read shape keeps it. Only this
// direction needs translating, and getting it wrong would drop the whole policy
// with a 201 and no error anywhere.
func retryPolicyToBody(p *retryPolicy) any {
	if p == nil {
		return nil
	}
	body := map[string]any{}
	if !p.MaxRetries.IsNull() && !p.MaxRetries.IsUnknown() {
		body["maxRetries"] = p.MaxRetries.ValueInt64()
	}
	if !p.InitialIntervalMs.IsNull() && !p.InitialIntervalMs.IsUnknown() {
		body["initialInterval"] = p.InitialIntervalMs.ValueInt64()
	}
	if !p.BackoffStrategy.IsNull() && !p.BackoffStrategy.IsUnknown() {
		body["backoffStrategy"] = p.BackoffStrategy.ValueString()
	}
	if !p.BackoffMultiplier.IsNull() && !p.BackoffMultiplier.IsUnknown() {
		body["backoffMultiplier"] = p.BackoffMultiplier.ValueFloat64()
	}
	if !p.MaxIntervalMs.IsNull() && !p.MaxIntervalMs.IsUnknown() {
		body["maxInterval"] = p.MaxIntervalMs.ValueInt64()
	}
	if !p.Jitter.IsNull() && !p.Jitter.IsUnknown() {
		body["jitter"] = p.Jitter.ValueBool()
	}
	return body
}

func destinationAPIToModel(ctx context.Context, d destinationAPIResponse, projectID string) (destinationModel, diag.Diagnostics) {
	headersMap, diags := types.MapValueFrom(ctx, types.StringType, d.Headers)

	effective, effectiveDiags := types.ObjectValue(effectiveRetryPolicyAttrTypes, map[string]attr.Value{
		"max_retries":         types.Int64Value(d.EffectiveRetryPolicy.MaxRetries),
		"initial_interval_ms": types.Int64Value(d.EffectiveRetryPolicy.InitialIntervalMs),
		"backoff_strategy":    types.StringValue(d.EffectiveRetryPolicy.BackoffStrategy),
		"backoff_multiplier":  types.Float64Value(d.EffectiveRetryPolicy.BackoffMultiplier),
		"max_interval_ms":     types.Int64Value(d.EffectiveRetryPolicy.MaxIntervalMs),
		"jitter":              types.BoolValue(d.EffectiveRetryPolicy.Jitter),
		"timeout_ms":          types.Int64Value(d.EffectiveRetryPolicy.TimeoutMs),
	})
	diags.Append(effectiveDiags...)

	return destinationModel{
		ID:                   types.StringValue(d.ID),
		ProjectID:            types.StringValue(projectID),
		EndpointID:           types.StringValue(d.EndpointID),
		Name:                 types.StringValue(d.Name),
		URL:                  types.StringValue(d.URL),
		Method:               types.StringValue(d.Method),
		Headers:              headersMap,
		ContentType:          types.StringValue(d.ContentType),
		TimeoutMs:            int64OrNull(d.TimeoutMs),
		RetryPolicy:          retryPolicyFromAPI(d.RetryPolicy),
		EffectiveRetryPolicy: effective,
		IsEnabled:            types.BoolValue(d.IsEnabled),
		CreatedAt:            types.StringValue(d.CreatedAt),
		UpdatedAt:            types.StringValue(d.UpdatedAt),
	}, diags
}

// A destination that pins nothing reads back as an absent block rather than a
// block of nulls, so a config that omits `retry_policy` plans clean instead of
// diffing against six explicit nulls forever.
func retryPolicyFromAPI(p retryPolicyAPI) *retryPolicy {
	if p.MaxRetries == nil && p.InitialIntervalMs == nil && p.BackoffStrategy == nil &&
		p.BackoffMultiplier == nil && p.MaxIntervalMs == nil && p.Jitter == nil {
		return nil
	}
	return &retryPolicy{
		MaxRetries:        int64OrNull(p.MaxRetries),
		InitialIntervalMs: int64OrNull(p.InitialIntervalMs),
		BackoffStrategy:   stringOrNull(p.BackoffStrategy),
		BackoffMultiplier: float64OrNull(p.BackoffMultiplier),
		MaxIntervalMs:     int64OrNull(p.MaxIntervalMs),
		Jitter:            boolOrNull(p.Jitter),
	}
}

func int64OrNull(v *int64) types.Int64 {
	if v == nil {
		return types.Int64Null()
	}
	return types.Int64Value(*v)
}

func stringOrNull(v *string) types.String {
	if v == nil {
		return types.StringNull()
	}
	return types.StringValue(*v)
}

func float64OrNull(v *float64) types.Float64 {
	if v == nil {
		return types.Float64Null()
	}
	return types.Float64Value(*v)
}

func boolOrNull(v *bool) types.Bool {
	if v == nil {
		return types.BoolNull()
	}
	return types.BoolValue(*v)
}

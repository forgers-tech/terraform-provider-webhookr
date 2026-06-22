package resource

import (
	"context"
	"fmt"
	"net/http"

	"github.com/forgers-tech/terraform-provider-webhookr/internal/client"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64default"
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
	ID          types.String `tfsdk:"id"`
	ProjectID   types.String `tfsdk:"project_id"`
	EndpointID  types.String `tfsdk:"endpoint_id"`
	Name        types.String `tfsdk:"name"`
	URL         types.String `tfsdk:"url"`
	Method      types.String `tfsdk:"method"`
	Headers     types.Map    `tfsdk:"headers"`
	ContentType types.String `tfsdk:"content_type"`
	TimeoutMs   types.Int64  `tfsdk:"timeout_ms"`
	IsEnabled   types.Bool   `tfsdk:"is_enabled"`
	CreatedAt   types.String `tfsdk:"created_at"`
	UpdatedAt   types.String `tfsdk:"updated_at"`
}

type destinationAPIResponse struct {
	ID          string            `json:"id"`
	EndpointID  string            `json:"endpointId"`
	Name        string            `json:"name"`
	URL         string            `json:"url"`
	Method      string            `json:"method"`
	Headers     map[string]string `json:"headers"`
	ContentType string            `json:"contentType"`
	TimeoutMs   int64             `json:"timeoutMs"`
	IsEnabled   bool              `json:"isEnabled"`
	CreatedAt   string            `json:"createdAt"`
	UpdatedAt   string            `json:"updatedAt"`
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
			"timeout_ms": schema.Int64Attribute{
				Optional:    true,
				Computed:    true,
				Default:     int64default.StaticInt64(30000),
				Description: "Request timeout in milliseconds (1000–60000).",
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
	if err != nil {
		resp.Diagnostics.AddError("Error reading destination", err.Error())
		return
	}
	if status == http.StatusNotFound {
		resp.State.RemoveResource(ctx)
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
	if err != nil {
		resp.Diagnostics.AddError("Error deleting destination", err.Error())
		return
	}
	if status == http.StatusNotFound {
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
		"timeoutMs":   m.TimeoutMs.ValueInt64(),
		"isEnabled":   m.IsEnabled.ValueBool(),
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

func destinationAPIToModel(ctx context.Context, d destinationAPIResponse, projectID string) (destinationModel, diag.Diagnostics) {
	headersMap, diags := types.MapValueFrom(ctx, types.StringType, d.Headers)
	return destinationModel{
		ID:          types.StringValue(d.ID),
		ProjectID:   types.StringValue(projectID),
		EndpointID:  types.StringValue(d.EndpointID),
		Name:        types.StringValue(d.Name),
		URL:         types.StringValue(d.URL),
		Method:      types.StringValue(d.Method),
		Headers:     headersMap,
		ContentType: types.StringValue(d.ContentType),
		TimeoutMs:   types.Int64Value(d.TimeoutMs),
		IsEnabled:   types.BoolValue(d.IsEnabled),
		CreatedAt:   types.StringValue(d.CreatedAt),
		UpdatedAt:   types.StringValue(d.UpdatedAt),
	}, diags
}

package resource

import (
	"context"
	"fmt"
	"net/http"

	"github.com/forgers-tech/terraform-provider-webhookr/internal/client"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ resource.Resource = (*ProjectResource)(nil)

type ProjectResource struct {
	client *client.Client
}

type projectModel struct {
	ID        types.String `tfsdk:"id"`
	Name      types.String `tfsdk:"name"`
	CreatedAt types.String `tfsdk:"created_at"`
	UpdatedAt types.String `tfsdk:"updated_at"`
}

type projectAPIResponse struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	CreatedAt string `json:"createdAt"`
	UpdatedAt string `json:"updatedAt"`
}

func NewProjectResource() resource.Resource {
	return &ProjectResource{}
}

func (r *ProjectResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_project"
}

func (r *ProjectResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a Webhookr project.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:    true,
				Description: "Unique identifier of the project.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				Required:    true,
				Description: "Display name of the project (max 100 characters).",
			},
			"created_at": schema.StringAttribute{
				Computed:    true,
				Description: "RFC3339 timestamp of when the project was created.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"updated_at": schema.StringAttribute{
				Computed:    true,
				Description: "RFC3339 timestamp of the last project update.",
			},
		},
	}
}

func (r *ProjectResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *ProjectResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan projectModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	body := map[string]string{"name": plan.Name.ValueString()}
	var result projectAPIResponse
	status, err := r.client.Do(ctx, http.MethodPost, "/v1/projects", body, &result)
	if err != nil {
		resp.Diagnostics.AddError("Error creating project", err.Error())
		return
	}
	if status != http.StatusCreated {
		resp.Diagnostics.AddError("Unexpected status creating project",
			fmt.Sprintf("expected 201, got %d", status))
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, apiToModel(result))...)
}

func (r *ProjectResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state projectModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var result projectAPIResponse
	status, err := r.client.Do(ctx, http.MethodGet, "/v1/projects/"+state.ID.ValueString(), nil, &result)
	if err != nil {
		resp.Diagnostics.AddError("Error reading project", err.Error())
		return
	}
	if status == http.StatusNotFound {
		resp.State.RemoveResource(ctx)
		return
	}
	if status != http.StatusOK {
		resp.Diagnostics.AddError("Unexpected status reading project",
			fmt.Sprintf("expected 200, got %d", status))
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, apiToModel(result))...)
}

func (r *ProjectResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan projectModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var state projectModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	body := map[string]string{"name": plan.Name.ValueString()}
	var result projectAPIResponse
	status, err := r.client.Do(ctx, http.MethodPatch, "/v1/projects/"+state.ID.ValueString(), body, &result)
	if err != nil {
		resp.Diagnostics.AddError("Error updating project", err.Error())
		return
	}
	if status != http.StatusOK {
		resp.Diagnostics.AddError("Unexpected status updating project",
			fmt.Sprintf("expected 200, got %d", status))
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, apiToModel(result))...)
}

func (r *ProjectResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state projectModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	status, err := r.client.Do(ctx, http.MethodDelete, "/v1/projects/"+state.ID.ValueString(), nil, nil)
	if err != nil {
		resp.Diagnostics.AddError("Error deleting project", err.Error())
		return
	}
	if status == http.StatusNotFound {
		return
	}
	if status != http.StatusOK {
		resp.Diagnostics.AddError("Unexpected status deleting project",
			fmt.Sprintf("expected 200, got %d", status))
		return
	}
}

func apiToModel(p projectAPIResponse) projectModel {
	return projectModel{
		ID:        types.StringValue(p.ID),
		Name:      types.StringValue(p.Name),
		CreatedAt: types.StringValue(p.CreatedAt),
		UpdatedAt: types.StringValue(p.UpdatedAt),
	}
}

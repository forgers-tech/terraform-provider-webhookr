package resource_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/forgers-tech/terraform-provider-webhookr/internal/client"
	tfresource "github.com/forgers-tech/terraform-provider-webhookr/internal/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource"
)

func testProject() map[string]interface{} {
	return map[string]interface{}{
		"id":        "proj-123",
		"name":      "My Project",
		"createdAt": "2024-01-01T00:00:00.000Z",
		"updatedAt": "2024-01-01T00:00:00.000Z",
	}
}

func TestProjectResource_Metadata(t *testing.T) {
	r := tfresource.NewProjectResource()
	metaReq := resource.MetadataRequest{ProviderTypeName: "webhookr"}
	var metaResp resource.MetadataResponse
	r.Metadata(context.Background(), metaReq, &metaResp)

	if metaResp.TypeName != "webhookr_project" {
		t.Errorf("expected type name webhookr_project, got %s", metaResp.TypeName)
	}
}

func TestProjectResource_Schema(t *testing.T) {
	r := tfresource.NewProjectResource()
	schemaReq := resource.SchemaRequest{}
	var schemaResp resource.SchemaResponse
	r.Schema(context.Background(), schemaReq, &schemaResp)

	attrs := schemaResp.Schema.Attributes
	for _, attr := range []string{"id", "name", "created_at", "updated_at"} {
		if _, ok := attrs[attr]; !ok {
			t.Errorf("missing attribute: %s", attr)
		}
	}
}

func TestNewProjectResource(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/v1/projects":
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(testProject())
		case r.Method == http.MethodGet && r.URL.Path == "/v1/projects/proj-123":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(testProject())
		case r.Method == http.MethodPatch && r.URL.Path == "/v1/projects/proj-123":
			w.Header().Set("Content-Type", "application/json")
			updated := testProject()
			updated["name"] = "Updated Project"
			_ = json.NewEncoder(w).Encode(updated)
		case r.Method == http.MethodDelete && r.URL.Path == "/v1/projects/proj-123":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(testProject())
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	staticToken := staticTokener("test-token")
	c := client.New(srv.URL, staticToken)

	r := tfresource.NewProjectResource()
	configurable, ok := r.(resource.ResourceWithConfigure)
	if !ok {
		t.Fatal("ProjectResource does not implement ResourceWithConfigure")
	}
	configReq := resource.ConfigureRequest{ProviderData: c}
	var configResp resource.ConfigureResponse
	configurable.Configure(context.Background(), configReq, &configResp)
	if configResp.Diagnostics.HasError() {
		t.Fatalf("Configure failed: %v", configResp.Diagnostics)
	}
}

type staticTokener string

func (s staticTokener) Token(_ context.Context) (string, error) {
	return string(s), nil
}

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

func testEndpoint() map[string]interface{} {
	return map[string]interface{}{
		"id":        "ep-456",
		"projectId": "proj-123",
		"name":      "Stripe Payments",
		"slug":      "ab1234cdef",
		"isActive":  true,
		"createdAt": "2024-01-01T00:00:00.000Z",
		"updatedAt": "2024-01-01T00:00:00.000Z",
	}
}

func TestEndpointResource_Metadata(t *testing.T) {
	r := tfresource.NewEndpointResource()
	metaReq := resource.MetadataRequest{ProviderTypeName: "webhookr"}
	var metaResp resource.MetadataResponse
	r.Metadata(context.Background(), metaReq, &metaResp)

	if metaResp.TypeName != "webhookr_endpoint" {
		t.Errorf("expected type name webhookr_endpoint, got %s", metaResp.TypeName)
	}
}

func TestEndpointResource_Schema(t *testing.T) {
	r := tfresource.NewEndpointResource()
	schemaReq := resource.SchemaRequest{}
	var schemaResp resource.SchemaResponse
	r.Schema(context.Background(), schemaReq, &schemaResp)

	attrs := schemaResp.Schema.Attributes
	for _, attr := range []string{"id", "project_id", "name", "slug", "is_active", "created_at", "updated_at"} {
		if _, ok := attrs[attr]; !ok {
			t.Errorf("missing attribute: %s", attr)
		}
	}
}

func TestEndpointResource_Configure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/v1/projects/proj-123/endpoints":
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(testEndpoint())
		case r.Method == http.MethodGet && r.URL.Path == "/v1/projects/proj-123/endpoints/ep-456":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(testEndpoint())
		case r.Method == http.MethodPatch && r.URL.Path == "/v1/projects/proj-123/endpoints/ep-456":
			w.Header().Set("Content-Type", "application/json")
			updated := testEndpoint()
			updated["name"] = "Updated Payments"
			_ = json.NewEncoder(w).Encode(updated)
		case r.Method == http.MethodDelete && r.URL.Path == "/v1/projects/proj-123/endpoints/ep-456":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(testEndpoint())
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	c := client.New(srv.URL, staticTokener("test-token"))

	r := tfresource.NewEndpointResource()
	configurable, ok := r.(resource.ResourceWithConfigure)
	if !ok {
		t.Fatal("EndpointResource does not implement ResourceWithConfigure")
	}
	configReq := resource.ConfigureRequest{ProviderData: c}
	var configResp resource.ConfigureResponse
	configurable.Configure(context.Background(), configReq, &configResp)
	if configResp.Diagnostics.HasError() {
		t.Fatalf("Configure failed: %v", configResp.Diagnostics)
	}
}

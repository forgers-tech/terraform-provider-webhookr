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

func testDestination() map[string]interface{} {
	return map[string]interface{}{
		"id":          "dst-789",
		"endpointId":  "ep-456",
		"name":        "My Webhook Receiver",
		"url":         "https://example.com/webhook",
		"method":      "POST",
		"headers":     map[string]interface{}{"X-API-Key": "secret"},
		"contentType": "application/json",
		"timeoutMs":   float64(30000),
		"isEnabled":   true,
		"createdAt":   "2024-01-01T00:00:00.000Z",
		"updatedAt":   "2024-01-01T00:00:00.000Z",
	}
}

func TestDestinationResource_Metadata(t *testing.T) {
	r := tfresource.NewDestinationResource()
	metaReq := resource.MetadataRequest{ProviderTypeName: "webhookr"}
	var metaResp resource.MetadataResponse
	r.Metadata(context.Background(), metaReq, &metaResp)

	if metaResp.TypeName != "webhookr_destination" {
		t.Errorf("expected type name webhookr_destination, got %s", metaResp.TypeName)
	}
}

func TestDestinationResource_Schema(t *testing.T) {
	r := tfresource.NewDestinationResource()
	schemaReq := resource.SchemaRequest{}
	var schemaResp resource.SchemaResponse
	r.Schema(context.Background(), schemaReq, &schemaResp)

	attrs := schemaResp.Schema.Attributes
	for _, attr := range []string{
		"id", "project_id", "endpoint_id", "name", "url",
		"method", "headers", "content_type", "timeout_ms", "is_enabled",
		"created_at", "updated_at",
	} {
		if _, ok := attrs[attr]; !ok {
			t.Errorf("missing attribute: %s", attr)
		}
	}
}

func TestDestinationResource_Configure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		base := "/v1/projects/proj-123/endpoints/ep-456/destinations"
		switch {
		case r.Method == http.MethodPost && r.URL.Path == base:
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(testDestination())
		case r.Method == http.MethodGet && r.URL.Path == base+"/dst-789":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(testDestination())
		case r.Method == http.MethodPatch && r.URL.Path == base+"/dst-789":
			w.Header().Set("Content-Type", "application/json")
			updated := testDestination()
			updated["name"] = "Updated Receiver"
			_ = json.NewEncoder(w).Encode(updated)
		case r.Method == http.MethodDelete && r.URL.Path == base+"/dst-789":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(testDestination())
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	c := client.New(srv.URL, staticTokener("test-token"))

	r := tfresource.NewDestinationResource()
	configurable, ok := r.(resource.ResourceWithConfigure)
	if !ok {
		t.Fatal("DestinationResource does not implement ResourceWithConfigure")
	}
	configReq := resource.ConfigureRequest{ProviderData: c}
	var configResp resource.ConfigureResponse
	configurable.Configure(context.Background(), configReq, &configResp)
	if configResp.Diagnostics.HasError() {
		t.Fatalf("Configure failed: %v", configResp.Diagnostics)
	}
}

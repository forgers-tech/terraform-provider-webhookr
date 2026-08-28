package resource_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/forgers-tech/terraform-provider-webhookr/internal/client"
	tfresource "github.com/forgers-tech/terraform-provider-webhookr/internal/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

// client.Do returns a status AND an error for anything >= 400, so a resource
// that checks the error first can never reach its 404 branch. That mistake is
// invisible until something is deleted outside Terraform, at which point the
// next plan fails instead of re-creating it — so every resource is pinned here
// rather than only the one that happened to be written last.

// Builds a state value for an arbitrary schema: every string attribute gets a
// placeholder, everything else is null. Enough for the resource to decode its
// state and build a request path.
func placeholderState(t *testing.T, s schema.Schema) tftypes.Value {
	t.Helper()
	objType, ok := s.Type().TerraformType(context.Background()).(tftypes.Object)
	if !ok {
		t.Fatal("schema type is not an object")
	}
	values := map[string]tftypes.Value{}
	for name, attrType := range objType.AttributeTypes {
		if attrType.Is(tftypes.String) {
			values[name] = tftypes.NewValue(tftypes.String, "placeholder")
			continue
		}
		values[name] = tftypes.NewValue(attrType, nil)
	}
	return tftypes.NewValue(objType, values)
}

func alwaysNotFound(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"message":"Not Found"}`))
	}))
}

func configuredResource(
	t *testing.T,
	newResource func() resource.Resource,
	srv *httptest.Server,
) (resource.Resource, schema.Schema) {
	t.Helper()
	r := newResource()

	var schemaResp resource.SchemaResponse
	r.Schema(context.Background(), resource.SchemaRequest{}, &schemaResp)

	configurable, ok := r.(resource.ResourceWithConfigure)
	if !ok {
		t.Fatal("resource does not implement ResourceWithConfigure")
	}
	var configResp resource.ConfigureResponse
	configurable.Configure(
		context.Background(),
		resource.ConfigureRequest{ProviderData: client.New(srv.URL, staticTokener("test-token"))},
		&configResp,
	)
	if configResp.Diagnostics.HasError() {
		t.Fatalf("Configure failed: %v", configResp.Diagnostics)
	}
	return r, schemaResp.Schema
}

var resourcesUnderTest = map[string]func() resource.Resource{
	"webhookr_project":                        tfresource.NewProjectResource,
	"webhookr_endpoint":                       tfresource.NewEndpointResource,
	"webhookr_endpoint_hmac":                  tfresource.NewEndpointHmacResource,
	"webhookr_endpoint_provider_verification": tfresource.NewEndpointProviderVerificationResource,
	"webhookr_destination":                    tfresource.NewDestinationResource,
}

func TestRead_RemovesResourceOnNotFound(t *testing.T) {
	for name, newResource := range resourcesUnderTest {
		t.Run(name, func(t *testing.T) {
			srv := alwaysNotFound(t)
			defer srv.Close()

			r, s := configuredResource(t, newResource, srv)
			state := placeholderState(t, s)

			var readResp resource.ReadResponse
			readResp.State = tfsdk.State{Schema: s, Raw: state}
			r.Read(context.Background(), resource.ReadRequest{
				State: tfsdk.State{Schema: s, Raw: state},
			}, &readResp)

			if readResp.Diagnostics.HasError() {
				t.Fatalf("a 404 must not be an error, it means the resource is gone: %v",
					readResp.Diagnostics)
			}
			if !readResp.State.Raw.IsNull() {
				t.Error("expected the resource to be removed from state")
			}
		})
	}
}

func TestDelete_SucceedsOnNotFound(t *testing.T) {
	// Already gone is the outcome Delete wanted, so it must not be an error.
	for name, newResource := range resourcesUnderTest {
		t.Run(name, func(t *testing.T) {
			srv := alwaysNotFound(t)
			defer srv.Close()

			r, s := configuredResource(t, newResource, srv)

			var deleteResp resource.DeleteResponse
			deleteResp.State = tfsdk.State{Schema: s}
			r.Delete(context.Background(), resource.DeleteRequest{
				State: tfsdk.State{Schema: s, Raw: placeholderState(t, s)},
			}, &deleteResp)

			if deleteResp.Diagnostics.HasError() {
				t.Errorf("deleting an already-deleted resource must succeed: %v",
					deleteResp.Diagnostics)
			}
		})
	}
}

package resource_test

import (
	"context"
	"encoding/json"
	"io"
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

const (
	hmacProjectID  = "proj-123"
	hmacEndpointID = "ep-456"
	hmacPath       = "/v1/projects/proj-123/endpoints/ep-456/hmac"
	endpointGetPat = "/v1/projects/proj-123/endpoints/ep-456"
)

func hmacResource(t *testing.T, srv *httptest.Server) (resource.Resource, schema.Schema) {
	t.Helper()
	r := tfresource.NewEndpointHmacResource()

	var schemaResp resource.SchemaResponse
	r.Schema(context.Background(), resource.SchemaRequest{}, &schemaResp)

	configurable, ok := r.(resource.ResourceWithConfigure)
	if !ok {
		t.Fatal("EndpointHmacResource does not implement ResourceWithConfigure")
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

// hmacValue builds the tftypes value for a plan or state. A nil entry becomes a
// null attribute, which is how Terraform represents "not set in configuration".
func hmacValue(t *testing.T, s schema.Schema, attrs map[string]*string) tftypes.Value {
	t.Helper()
	objType, ok := s.Type().TerraformType(context.Background()).(tftypes.Object)
	if !ok {
		t.Fatal("schema type is not an object")
	}
	values := map[string]tftypes.Value{}
	for name := range objType.AttributeTypes {
		if v, present := attrs[name]; present && v != nil {
			values[name] = tftypes.NewValue(tftypes.String, *v)
			continue
		}
		values[name] = tftypes.NewValue(tftypes.String, nil)
	}
	return tftypes.NewValue(objType, values)
}

func str(v string) *string { return &v }

func hmacAPIBody(secret, headerName string) map[string]interface{} {
	return map[string]interface{}{
		"enabled":    true,
		"algorithm":  "sha256",
		"headerName": headerName,
		"keyId":      "hk_deadbeef",
		"secret":     secret,
	}
}

func TestEndpointHmacResource_Metadata(t *testing.T) {
	r := tfresource.NewEndpointHmacResource()
	var metaResp resource.MetadataResponse
	r.Metadata(context.Background(), resource.MetadataRequest{ProviderTypeName: "webhookr"}, &metaResp)

	if metaResp.TypeName != "webhookr_endpoint_hmac" {
		t.Errorf("expected type name webhookr_endpoint_hmac, got %s", metaResp.TypeName)
	}
}

func TestEndpointHmacResource_Schema(t *testing.T) {
	r := tfresource.NewEndpointHmacResource()
	var schemaResp resource.SchemaResponse
	r.Schema(context.Background(), resource.SchemaRequest{}, &schemaResp)

	attrs := schemaResp.Schema.Attributes
	for _, attr := range []string{"project_id", "endpoint_id", "header_name", "secret", "algorithm", "key_id"} {
		if _, ok := attrs[attr]; !ok {
			t.Errorf("missing attribute: %s", attr)
		}
	}
	if !attrs["secret"].IsSensitive() {
		t.Error("secret must be marked sensitive so it is not printed in plans or logs")
	}
}

func TestEndpointHmacResource_CreateStoresGeneratedSecret(t *testing.T) {
	var sentBody map[string]interface{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut || r.URL.Path != hmacPath {
			http.NotFound(w, r)
			return
		}
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &sentBody)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(hmacAPIBody("whsec_generated", "X-Webhookr-Signature"))
	}))
	defer srv.Close()

	r, s := hmacResource(t, srv)
	plan := hmacValue(t, s, map[string]*string{
		"project_id":  str(hmacProjectID),
		"endpoint_id": str(hmacEndpointID),
	})

	var createResp resource.CreateResponse
	createResp.State = tfsdk.State{Schema: s}
	r.Create(context.Background(), resource.CreateRequest{
		Plan: tfsdk.Plan{Schema: s, Raw: plan},
	}, &createResp)

	if createResp.Diagnostics.HasError() {
		t.Fatalf("Create failed: %v", createResp.Diagnostics)
	}
	if _, present := sentBody["secret"]; present {
		t.Error("Create must not send a secret when the configuration does not set one")
	}
	assertStateString(t, createResp.State, "secret", "whsec_generated")
	assertStateString(t, createResp.State, "key_id", "hk_deadbeef")
}

func TestEndpointHmacResource_CreateSendsSuppliedSecret(t *testing.T) {
	var sentBody map[string]interface{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &sentBody)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(hmacAPIBody("my-own-secret-value", "X-Custom"))
	}))
	defer srv.Close()

	r, s := hmacResource(t, srv)
	plan := hmacValue(t, s, map[string]*string{
		"project_id":  str(hmacProjectID),
		"endpoint_id": str(hmacEndpointID),
		"header_name": str("X-Custom"),
		"secret":      str("my-own-secret-value"),
	})

	var createResp resource.CreateResponse
	createResp.State = tfsdk.State{Schema: s}
	r.Create(context.Background(), resource.CreateRequest{
		Plan: tfsdk.Plan{Schema: s, Raw: plan},
	}, &createResp)

	if createResp.Diagnostics.HasError() {
		t.Fatalf("Create failed: %v", createResp.Diagnostics)
	}
	if sentBody["secret"] != "my-own-secret-value" {
		t.Errorf("expected the supplied secret to be sent, got %v", sentBody["secret"])
	}
	if sentBody["headerName"] != "X-Custom" {
		t.Errorf("expected the supplied header name to be sent, got %v", sentBody["headerName"])
	}
}

func TestEndpointHmacResource_UpdateKeepsTheExistingSecret(t *testing.T) {
	var sentBody map[string]interface{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &sentBody)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(hmacAPIBody("whsec_existing", "X-Renamed"))
	}))
	defer srv.Close()

	r, s := hmacResource(t, srv)

	state := hmacValue(t, s, map[string]*string{
		"project_id":  str(hmacProjectID),
		"endpoint_id": str(hmacEndpointID),
		"header_name": str("X-Webhookr-Signature"),
		"secret":      str("whsec_existing"),
		"algorithm":   str("sha256"),
		"key_id":      str("hk_old"),
	})
	plan := hmacValue(t, s, map[string]*string{
		"project_id":  str(hmacProjectID),
		"endpoint_id": str(hmacEndpointID),
		"header_name": str("X-Renamed"),
	})

	var updateResp resource.UpdateResponse
	updateResp.State = tfsdk.State{Schema: s}
	r.Update(context.Background(), resource.UpdateRequest{
		Plan:  tfsdk.Plan{Schema: s, Raw: plan},
		State: tfsdk.State{Schema: s, Raw: state},
	}, &updateResp)

	if updateResp.Diagnostics.HasError() {
		t.Fatalf("Update failed: %v", updateResp.Diagnostics)
	}
	if sentBody["secret"] != "whsec_existing" {
		t.Errorf("renaming the header must not re-key the endpoint; sent secret %v", sentBody["secret"])
	}
	assertStateString(t, updateResp.State, "header_name", "X-Renamed")
}

func TestEndpointHmacResource_ReadDropsTheResourceWhenVerificationWasTurnedOff(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != endpointGetPat {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"hmac": map[string]interface{}{"enabled": false, "algorithm": "sha256"},
		})
	}))
	defer srv.Close()

	r, s := hmacResource(t, srv)
	state := hmacValue(t, s, map[string]*string{
		"project_id":  str(hmacProjectID),
		"endpoint_id": str(hmacEndpointID),
		"secret":      str("whsec_existing"),
	})

	var readResp resource.ReadResponse
	readResp.State = tfsdk.State{Schema: s, Raw: state}
	r.Read(context.Background(), resource.ReadRequest{
		State: tfsdk.State{Schema: s, Raw: state},
	}, &readResp)

	if readResp.Diagnostics.HasError() {
		t.Fatalf("Read failed: %v", readResp.Diagnostics)
	}
	if !readResp.State.Raw.IsNull() {
		t.Error("expected the resource to be removed from state after verification was disabled")
	}
}

func TestEndpointHmacResource_ReadKeepsTheSecretTheAPINeverReturns(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"hmac": map[string]interface{}{
				"enabled":    true,
				"algorithm":  "sha256",
				"headerName": "X-Webhookr-Signature",
				"keyId":      "hk_deadbeef",
			},
		})
	}))
	defer srv.Close()

	r, s := hmacResource(t, srv)
	state := hmacValue(t, s, map[string]*string{
		"project_id":  str(hmacProjectID),
		"endpoint_id": str(hmacEndpointID),
		"header_name": str("X-Old"),
		"secret":      str("whsec_existing"),
		"algorithm":   str("sha256"),
		"key_id":      str("hk_old"),
	})

	var readResp resource.ReadResponse
	readResp.State = tfsdk.State{Schema: s, Raw: state}
	r.Read(context.Background(), resource.ReadRequest{
		State: tfsdk.State{Schema: s, Raw: state},
	}, &readResp)

	if readResp.Diagnostics.HasError() {
		t.Fatalf("Read failed: %v", readResp.Diagnostics)
	}
	assertStateString(t, readResp.State, "secret", "whsec_existing")
	assertStateString(t, readResp.State, "header_name", "X-Webhookr-Signature")
	assertStateString(t, readResp.State, "key_id", "hk_deadbeef")
}

func TestEndpointHmacResource_DeleteDisablesVerification(t *testing.T) {
	var method, path string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		method, path = r.Method, r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"enabled": false})
	}))
	defer srv.Close()

	r, s := hmacResource(t, srv)
	state := hmacValue(t, s, map[string]*string{
		"project_id":  str(hmacProjectID),
		"endpoint_id": str(hmacEndpointID),
	})

	var deleteResp resource.DeleteResponse
	deleteResp.State = tfsdk.State{Schema: s, Raw: state}
	r.Delete(context.Background(), resource.DeleteRequest{
		State: tfsdk.State{Schema: s, Raw: state},
	}, &deleteResp)

	if deleteResp.Diagnostics.HasError() {
		t.Fatalf("Delete failed: %v", deleteResp.Diagnostics)
	}
	if method != http.MethodDelete || path != hmacPath {
		t.Errorf("expected DELETE %s, got %s %s", hmacPath, method, path)
	}
}

func assertStateString(t *testing.T, state tfsdk.State, attribute, expected string) {
	t.Helper()
	obj, ok := state.Raw.Type().(tftypes.Object)
	if !ok {
		t.Fatal("state is not an object")
	}
	values := map[string]tftypes.Value{}
	if err := state.Raw.As(&values); err != nil {
		t.Fatalf("reading state: %v", err)
	}
	if _, present := obj.AttributeTypes[attribute]; !present {
		t.Fatalf("unknown attribute %q", attribute)
	}
	var actual string
	if err := values[attribute].As(&actual); err != nil {
		t.Fatalf("reading %s: %v", attribute, err)
	}
	if actual != expected {
		t.Errorf("expected %s = %q, got %q", attribute, expected, actual)
	}
}

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
	providerVerificationPath = "/v1/projects/proj-123/endpoints/ep-456/verification/provider"
	stripeSigningSecret      = "whsec_test-stripe-signing-secret"
)

func providerVerificationResource(t *testing.T, srv *httptest.Server) (resource.Resource, schema.Schema) {
	t.Helper()
	r := tfresource.NewEndpointProviderVerificationResource()

	var schemaResp resource.SchemaResponse
	r.Schema(context.Background(), resource.SchemaRequest{}, &schemaResp)

	configurable, ok := r.(resource.ResourceWithConfigure)
	if !ok {
		t.Fatal("EndpointProviderVerificationResource does not implement ResourceWithConfigure")
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

func providerVerificationValue(t *testing.T, s schema.Schema, attrs map[string]*string) tftypes.Value {
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

func stripePlan(t *testing.T, s schema.Schema, secret string) tftypes.Value {
	t.Helper()
	return providerVerificationValue(t, s, map[string]*string{
		"project_id":    str(hmacProjectID),
		"endpoint_id":   str(hmacEndpointID),
		"provider_name": str("stripe"),
		"secret":        str(secret),
	})
}

func TestEndpointProviderVerificationResource_Metadata(t *testing.T) {
	r := tfresource.NewEndpointProviderVerificationResource()
	var metaResp resource.MetadataResponse
	r.Metadata(context.Background(), resource.MetadataRequest{ProviderTypeName: "webhookr"}, &metaResp)

	if metaResp.TypeName != "webhookr_endpoint_provider_verification" {
		t.Errorf("expected webhookr_endpoint_provider_verification, got %s", metaResp.TypeName)
	}
}

func TestEndpointProviderVerificationResource_Schema(t *testing.T) {
	r := tfresource.NewEndpointProviderVerificationResource()
	var schemaResp resource.SchemaResponse
	r.Schema(context.Background(), resource.SchemaRequest{}, &schemaResp)

	attrs := schemaResp.Schema.Attributes
	for _, attr := range []string{"project_id", "endpoint_id", "provider_name", "secret"} {
		if _, ok := attrs[attr]; !ok {
			t.Errorf("missing attribute: %s", attr)
		}
	}
	// `provider` is a reserved meta-argument in every Terraform resource block;
	// an attribute by that name would never reach this schema.
	if _, ok := attrs["provider"]; ok {
		t.Error("attribute must not be named provider — Terraform reserves it")
	}
	if !attrs["secret"].IsSensitive() {
		t.Error("secret must be marked sensitive so it is not printed in plans or logs")
	}
	if !attrs["secret"].IsRequired() {
		t.Error("secret must be required — Webhookr never generates a provider signing secret")
	}
}

func TestEndpointProviderVerificationResource_CreateSendsProviderAndSecret(t *testing.T) {
	var sentBody map[string]interface{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut || r.URL.Path != providerVerificationPath {
			http.NotFound(w, r)
			return
		}
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &sentBody)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"type": "provider", "provider": "stripe"})
	}))
	defer srv.Close()

	r, s := providerVerificationResource(t, srv)

	var createResp resource.CreateResponse
	createResp.State = tfsdk.State{Schema: s}
	r.Create(context.Background(), resource.CreateRequest{
		Plan: tfsdk.Plan{Schema: s, Raw: stripePlan(t, s, stripeSigningSecret)},
	}, &createResp)

	if createResp.Diagnostics.HasError() {
		t.Fatalf("Create failed: %v", createResp.Diagnostics)
	}
	if sentBody["provider"] != "stripe" {
		t.Errorf("expected provider stripe, got %v", sentBody["provider"])
	}
	if sentBody["secret"] != stripeSigningSecret {
		t.Errorf("expected the configured secret to be sent, got %v", sentBody["secret"])
	}
	assertStateString(t, createResp.State, "provider_name", "stripe")
	// The API returns no secret, so state must keep the configured value.
	assertStateString(t, createResp.State, "secret", stripeSigningSecret)
}

func TestEndpointProviderVerificationResource_CreateSurfacesHmacConflict(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusConflict)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"message": "conflict"})
	}))
	defer srv.Close()

	r, s := providerVerificationResource(t, srv)

	var createResp resource.CreateResponse
	createResp.State = tfsdk.State{Schema: s}
	r.Create(context.Background(), resource.CreateRequest{
		Plan: tfsdk.Plan{Schema: s, Raw: stripePlan(t, s, stripeSigningSecret)},
	}, &createResp)

	if !createResp.Diagnostics.HasError() {
		t.Fatal("expected a 409 to surface as an error")
	}
	summary := createResp.Diagnostics.Errors()[0].Summary()
	if summary != "Endpoint already verifies a generic HMAC signature" {
		t.Errorf("expected the conflict to be explained, got %q", summary)
	}
}

func TestEndpointProviderVerificationResource_UpdateReplacesTheSecret(t *testing.T) {
	const rotated = "whsec_test-stripe-rotated-secret"
	var sentBody map[string]interface{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &sentBody)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"type": "provider", "provider": "stripe"})
	}))
	defer srv.Close()

	r, s := providerVerificationResource(t, srv)

	var updateResp resource.UpdateResponse
	updateResp.State = tfsdk.State{Schema: s}
	r.Update(context.Background(), resource.UpdateRequest{
		Plan:  tfsdk.Plan{Schema: s, Raw: stripePlan(t, s, rotated)},
		State: tfsdk.State{Schema: s, Raw: stripePlan(t, s, stripeSigningSecret)},
	}, &updateResp)

	if updateResp.Diagnostics.HasError() {
		t.Fatalf("Update failed: %v", updateResp.Diagnostics)
	}
	if sentBody["secret"] != rotated {
		t.Errorf("expected the rotated secret to be sent, got %v", sentBody["secret"])
	}
	assertStateString(t, updateResp.State, "secret", rotated)
}

func TestEndpointProviderVerificationResource_ReadKeepsTheStoredSecret(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != endpointGetPat {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"verification": map[string]interface{}{"type": "provider", "provider": "stripe"},
		})
	}))
	defer srv.Close()

	r, s := providerVerificationResource(t, srv)

	var readResp resource.ReadResponse
	readResp.State = tfsdk.State{Schema: s}
	r.Read(context.Background(), resource.ReadRequest{
		State: tfsdk.State{Schema: s, Raw: stripePlan(t, s, stripeSigningSecret)},
	}, &readResp)

	if readResp.Diagnostics.HasError() {
		t.Fatalf("Read failed: %v", readResp.Diagnostics)
	}
	assertStateString(t, readResp.State, "provider_name", "stripe")
	assertStateString(t, readResp.State, "secret", stripeSigningSecret)
}

func TestEndpointProviderVerificationResource_ReadDropsResourceWhenStrategyChanged(t *testing.T) {
	for _, verification := range []map[string]interface{}{
		{"type": "none", "provider": nil},
		{"type": "hmac", "provider": nil},
	} {
		body := map[string]interface{}{"verification": verification}
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(body)
		}))

		r, s := providerVerificationResource(t, srv)

		var readResp resource.ReadResponse
		readResp.State = tfsdk.State{Schema: s, Raw: stripePlan(t, s, stripeSigningSecret)}
		r.Read(context.Background(), resource.ReadRequest{
			State: tfsdk.State{Schema: s, Raw: stripePlan(t, s, stripeSigningSecret)},
		}, &readResp)

		if readResp.Diagnostics.HasError() {
			t.Fatalf("Read failed: %v", readResp.Diagnostics)
		}
		if !readResp.State.Raw.IsNull() {
			t.Errorf("expected the resource to be removed from state for type %v", verification["type"])
		}
		srv.Close()
	}
}

func TestEndpointProviderVerificationResource_DeleteDisablesVerification(t *testing.T) {
	var called bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete || r.URL.Path != providerVerificationPath {
			http.NotFound(w, r)
			return
		}
		called = true
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"type": "none", "provider": nil})
	}))
	defer srv.Close()

	r, s := providerVerificationResource(t, srv)

	var deleteResp resource.DeleteResponse
	deleteResp.State = tfsdk.State{Schema: s}
	r.Delete(context.Background(), resource.DeleteRequest{
		State: tfsdk.State{Schema: s, Raw: stripePlan(t, s, stripeSigningSecret)},
	}, &deleteResp)

	if deleteResp.Diagnostics.HasError() {
		t.Fatalf("Delete failed: %v", deleteResp.Diagnostics)
	}
	if !called {
		t.Error("Delete must call DELETE on the provider verification sub-resource")
	}
}

func TestEndpointProviderVerificationResource_ValidateConfigRejectsUnsupportedProvider(t *testing.T) {
	r := tfresource.NewEndpointProviderVerificationResource()
	var schemaResp resource.SchemaResponse
	r.Schema(context.Background(), resource.SchemaRequest{}, &schemaResp)
	s := schemaResp.Schema

	validatable, ok := r.(resource.ResourceWithValidateConfig)
	if !ok {
		t.Fatal("resource does not implement ResourceWithValidateConfig")
	}

	config := providerVerificationValue(t, s, map[string]*string{
		"project_id":    str(hmacProjectID),
		"endpoint_id":   str(hmacEndpointID),
		"provider_name": str("shopify"),
		"secret":        str(stripeSigningSecret),
	})

	var validateResp resource.ValidateConfigResponse
	validatable.ValidateConfig(context.Background(), resource.ValidateConfigRequest{
		Config: tfsdk.Config{Schema: s, Raw: config},
	}, &validateResp)

	if !validateResp.Diagnostics.HasError() {
		t.Fatal("expected an unsupported provider to be rejected at plan time")
	}
}

func TestEndpointProviderVerificationResource_ValidateConfigAcceptsEverySupportedProvider(t *testing.T) {
	r := tfresource.NewEndpointProviderVerificationResource()
	var schemaResp resource.SchemaResponse
	r.Schema(context.Background(), resource.SchemaRequest{}, &schemaResp)
	s := schemaResp.Schema

	validatable, ok := r.(resource.ResourceWithValidateConfig)
	if !ok {
		t.Fatal("resource does not implement ResourceWithValidateConfig")
	}

	// Every name the provider claims to support must actually pass validation,
	// so the list and the check cannot drift apart.
	for _, name := range []string{"stripe", "github"} {
		config := providerVerificationValue(t, s, map[string]*string{
			"project_id":    str(hmacProjectID),
			"endpoint_id":   str(hmacEndpointID),
			"provider_name": str(name),
			"secret":        str(stripeSigningSecret),
		})

		var validateResp resource.ValidateConfigResponse
		validatable.ValidateConfig(context.Background(), resource.ValidateConfigRequest{
			Config: tfsdk.Config{Schema: s, Raw: config},
		}, &validateResp)

		if validateResp.Diagnostics.HasError() {
			t.Errorf("%s must be accepted: %v", name, validateResp.Diagnostics)
		}
	}
}

func TestEndpointProviderVerificationResource_ValidateConfigAcceptsStripe(t *testing.T) {
	r := tfresource.NewEndpointProviderVerificationResource()
	var schemaResp resource.SchemaResponse
	r.Schema(context.Background(), resource.SchemaRequest{}, &schemaResp)
	s := schemaResp.Schema

	validatable, ok := r.(resource.ResourceWithValidateConfig)
	if !ok {
		t.Fatal("resource does not implement ResourceWithValidateConfig")
	}

	var validateResp resource.ValidateConfigResponse
	validatable.ValidateConfig(context.Background(), resource.ValidateConfigRequest{
		Config: tfsdk.Config{Schema: s, Raw: stripePlan(t, s, stripeSigningSecret)},
	}, &validateResp)

	if validateResp.Diagnostics.HasError() {
		t.Fatalf("stripe must be accepted: %v", validateResp.Diagnostics)
	}
}

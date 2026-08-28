package provider_test

import (
	"context"
	"testing"

	tfprovider "github.com/forgers-tech/terraform-provider-webhookr/internal/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
)

func TestWebhookrProvider_Metadata(t *testing.T) {
	p := tfprovider.New("1.0.0")()
	var resp provider.MetadataResponse
	p.Metadata(context.Background(), provider.MetadataRequest{}, &resp)

	if resp.TypeName != "webhookr" {
		t.Errorf("expected TypeName %q, got %q", "webhookr", resp.TypeName)
	}
}

func TestWebhookrProvider_Schema(t *testing.T) {
	p := tfprovider.New("1.0.0")()
	var resp provider.SchemaResponse
	p.Schema(context.Background(), provider.SchemaRequest{}, &resp)

	required := []string{"api_url"}
	optional := []string{"api_token", "firebase_api_key", "service_account_email", "service_account_key"}
	sensitive := []string{"api_token", "firebase_api_key", "service_account_key"}

	attrs := resp.Schema.Attributes

	for _, name := range required {
		attr, ok := attrs[name]
		if !ok {
			t.Errorf("missing attribute: %s", name)
			continue
		}
		sa, ok := attr.(schema.StringAttribute)
		if !ok {
			t.Errorf("attribute %s is not a StringAttribute", name)
			continue
		}
		if !sa.Required {
			t.Errorf("attribute %s should be Required", name)
		}
	}

	for _, name := range optional {
		attr, ok := attrs[name]
		if !ok {
			t.Errorf("missing attribute: %s", name)
			continue
		}
		sa, ok := attr.(schema.StringAttribute)
		if !ok {
			t.Errorf("attribute %s is not a StringAttribute", name)
			continue
		}
		if !sa.Optional {
			t.Errorf("attribute %s should be Optional", name)
		}
	}

	for _, name := range sensitive {
		attr, ok := attrs[name]
		if !ok {
			t.Errorf("missing attribute: %s", name)
			continue
		}
		sa, ok := attr.(schema.StringAttribute)
		if !ok {
			t.Errorf("attribute %s is not a StringAttribute", name)
			continue
		}
		if !sa.Sensitive {
			t.Errorf("attribute %s should be Sensitive", name)
		}
	}
}

// A resource that compiles but is not returned here is unreachable from any
// configuration, and nothing else in the suite would notice.
func TestWebhookrProvider_Resources(t *testing.T) {
	p := tfprovider.New("1.0.0")()

	registered := map[string]bool{}
	for _, newResource := range p.Resources(context.Background()) {
		var resp resource.MetadataResponse
		newResource().Metadata(
			context.Background(),
			resource.MetadataRequest{ProviderTypeName: "webhookr"},
			&resp,
		)
		registered[resp.TypeName] = true
	}

	for _, name := range []string{
		"webhookr_project",
		"webhookr_endpoint",
		"webhookr_endpoint_hmac",
		"webhookr_endpoint_provider_verification",
		"webhookr_destination",
	} {
		if !registered[name] {
			t.Errorf("resource %s is not registered on the provider", name)
		}
	}
}

package provider_test

import (
	"context"
	"testing"

	tfprovider "github.com/forgers-tech/terraform-provider-webhookr/internal/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
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

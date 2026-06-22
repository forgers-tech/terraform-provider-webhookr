package provider

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/diag"
)

func containsSummary(diags diag.Diagnostics, summary string) bool {
	for _, d := range diags {
		if d.Summary() == summary {
			return true
		}
	}
	return false
}

func TestValidateAuthConfig_TokenOnly(t *testing.T) {
	diags := validateAuthConfig("whk_abc123", "", "", "")
	if diags.HasError() {
		t.Errorf("expected no error with api_token only, got: %v", diags)
	}
}

func TestValidateAuthConfig_FirebaseComplete(t *testing.T) {
	diags := validateAuthConfig("", "firebase-key", "sa@project.iam.gserviceaccount.com", "pem-key-value")
	if diags.HasError() {
		t.Errorf("expected no error with complete firebase config, got: %v", diags)
	}
}

func TestValidateAuthConfig_Conflict(t *testing.T) {
	diags := validateAuthConfig("whk_abc123", "firebase-key", "", "")
	if !diags.HasError() {
		t.Fatal("expected error for conflicting auth, got none")
	}
	if !containsSummary(diags, "Conflicting authentication") {
		t.Errorf("expected 'Conflicting authentication' diagnostic, got summaries: %v", diags)
	}
}

func TestValidateAuthConfig_NoAuth(t *testing.T) {
	diags := validateAuthConfig("", "", "", "")
	if !diags.HasError() {
		t.Fatal("expected error for missing auth, got none")
	}
	if !containsSummary(diags, "Missing authentication") {
		t.Errorf("expected 'Missing authentication' diagnostic, got summaries: %v", diags)
	}
}

func TestValidateAuthConfig_PartialFirebase_MissingAPIKey(t *testing.T) {
	diags := validateAuthConfig("", "", "sa@project.iam.gserviceaccount.com", "pem-key-value")
	if !diags.HasError() {
		t.Fatal("expected error for partial firebase config (missing api key), got none")
	}
	if !containsSummary(diags, "Missing firebase_api_key") {
		t.Errorf("expected 'Missing firebase_api_key' diagnostic, got: %v", diags)
	}
}

func TestValidateAuthConfig_PartialFirebase_MissingEmail(t *testing.T) {
	diags := validateAuthConfig("", "firebase-key", "", "pem-key-value")
	if !diags.HasError() {
		t.Fatal("expected error for partial firebase config (missing email), got none")
	}
	if !containsSummary(diags, "Missing service_account_email") {
		t.Errorf("expected 'Missing service_account_email' diagnostic, got: %v", diags)
	}
}

func TestValidateAuthConfig_PartialFirebase_MissingServiceKey(t *testing.T) {
	diags := validateAuthConfig("", "firebase-key", "sa@project.iam.gserviceaccount.com", "")
	if !diags.HasError() {
		t.Fatal("expected error for partial firebase config (missing service key), got none")
	}
	if !containsSummary(diags, "Missing service_account_key") {
		t.Errorf("expected 'Missing service_account_key' diagnostic, got: %v", diags)
	}
}

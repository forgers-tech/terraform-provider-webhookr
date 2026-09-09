package resource

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestRetryPolicyToBody_RenamesTheDurationFields(t *testing.T) {
	// The API's write shape drops the `Ms` suffix because it also accepts unit
	// strings such as "10s"; the read shape keeps it. A mismatch here would send
	// a policy the service silently discards — a 201 and no error anywhere — so
	// the exact key names are the assertion.
	body := retryPolicyToBody(&retryPolicy{
		MaxRetries:        types.Int64Value(3),
		InitialIntervalMs: types.Int64Value(20000),
		BackoffStrategy:   types.StringValue("exponential"),
		BackoffMultiplier: types.Float64Value(2),
		MaxIntervalMs:     types.Int64Value(120000),
		Jitter:            types.BoolValue(false),
	})

	got, ok := body.(map[string]any)
	if !ok {
		t.Fatalf("expected a map, got %T", body)
	}

	want := map[string]any{
		"maxRetries":        int64(3),
		"initialInterval":   int64(20000),
		"backoffStrategy":   "exponential",
		"backoffMultiplier": float64(2),
		"maxInterval":       int64(120000),
		"jitter":            false,
	}
	if len(got) != len(want) {
		t.Fatalf("expected %d keys, got %d: %v", len(want), len(got), got)
	}
	for key, expected := range want {
		if got[key] != expected {
			t.Errorf("%s: expected %v, got %v", key, expected, got[key])
		}
	}
}

func TestRetryPolicyToBody_SendsOnlyTheFieldsThatWereSet(t *testing.T) {
	// An omitted field must stay omitted so it inherits the platform default,
	// rather than being pinned to whatever zero value Go would serialise.
	body := retryPolicyToBody(&retryPolicy{
		MaxRetries:        types.Int64Value(2),
		InitialIntervalMs: types.Int64Null(),
		BackoffStrategy:   types.StringNull(),
		BackoffMultiplier: types.Float64Null(),
		MaxIntervalMs:     types.Int64Null(),
		Jitter:            types.BoolNull(),
	})

	got, ok := body.(map[string]any)
	if !ok {
		t.Fatalf("expected a map, got %T", body)
	}
	if len(got) != 1 || got["maxRetries"] != int64(2) {
		t.Errorf("expected only maxRetries, got %v", got)
	}
}

func TestRetryPolicyToBody_KeepsAnExplicitZeroAndFalse(t *testing.T) {
	// maxRetries 0 disables retries and jitter false turns it off. Treating
	// either as "unset" would silently restore the default.
	body := retryPolicyToBody(&retryPolicy{
		MaxRetries: types.Int64Value(0),
		Jitter:     types.BoolValue(false),
	})

	got := body.(map[string]any)
	if got["maxRetries"] != int64(0) {
		t.Errorf("expected maxRetries 0, got %v", got["maxRetries"])
	}
	if got["jitter"] != false {
		t.Errorf("expected jitter false, got %v", got["jitter"])
	}
}

func TestDestinationModelToBody_ClearsThePolicyWhenTheBlockIsAbsent(t *testing.T) {
	// Terraform configuration is the source of truth, so a policy removed from
	// the config has to be cleared. Omitting the key would leave whatever was
	// stored in place and the resource would never converge.
	body, diags := destinationModelToBody(context.Background(), baseModel())
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}

	value, present := body["retryPolicy"]
	if !present {
		t.Fatal("retryPolicy must be present so the service clears the stored policy")
	}
	if value != nil {
		t.Errorf("expected an explicit null, got %v", value)
	}
}

func TestDestinationModelToBody_SendsANullTimeoutRatherThanZero(t *testing.T) {
	// The zero value is below the API's minimum of 1000 and would be rejected;
	// null is what asks for the platform default.
	for name, model := range map[string]destinationModel{
		"null":    withTimeout(types.Int64Null()),
		"unknown": withTimeout(types.Int64Unknown()),
	} {
		t.Run(name, func(t *testing.T) {
			body, _ := destinationModelToBody(context.Background(), model)
			if body["timeoutMs"] != nil {
				t.Errorf("expected null, got %v", body["timeoutMs"])
			}
		})
	}
}

func TestDestinationModelToBody_SendsAConfiguredTimeout(t *testing.T) {
	body, _ := destinationModelToBody(context.Background(), withTimeout(types.Int64Value(5000)))

	if body["timeoutMs"] != int64(5000) {
		t.Errorf("expected 5000, got %v", body["timeoutMs"])
	}
}

func TestRetryPolicyFromAPI_ReadsAnUnpinnedDestinationAsAnAbsentBlock(t *testing.T) {
	// Otherwise a config that omits `retry_policy` would diff forever against a
	// block of six explicit nulls.
	if got := retryPolicyFromAPI(retryPolicyAPI{}); got != nil {
		t.Errorf("expected nil, got %+v", got)
	}
}

func TestRetryPolicyFromAPI_KeepsNullsInsideAPartiallyPinnedPolicy(t *testing.T) {
	maxRetries := int64(3)
	got := retryPolicyFromAPI(retryPolicyAPI{MaxRetries: &maxRetries})

	if got == nil {
		t.Fatal("expected a policy")
	}
	if got.MaxRetries.ValueInt64() != 3 {
		t.Errorf("expected maxRetries 3, got %v", got.MaxRetries)
	}
	// Null here means "follows the platform default", which is a different fact
	// from the number that default happens to be today.
	if !got.InitialIntervalMs.IsNull() {
		t.Errorf("expected initial_interval_ms to stay null, got %v", got.InitialIntervalMs)
	}
	if !got.Jitter.IsNull() {
		t.Errorf("expected jitter to stay null, got %v", got.Jitter)
	}
}

func TestDestinationAPIToModel_MapsTheEffectivePolicyAndANullTimeout(t *testing.T) {
	model, diags := destinationAPIToModel(context.Background(), destinationAPIResponse{
		ID:         "dst-789",
		EndpointID: "ep-456",
		EffectiveRetryPolicy: effectiveRetryPolicyAPI{
			MaxRetries:        5,
			InitialIntervalMs: 10000,
			BackoffStrategy:   "exponential",
			BackoffMultiplier: 2,
			MaxIntervalMs:     300000,
			Jitter:            true,
			TimeoutMs:         30000,
		},
	}, "proj-123")
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}

	if !model.TimeoutMs.IsNull() {
		t.Errorf("expected a null timeout, got %v", model.TimeoutMs)
	}
	if model.RetryPolicy != nil {
		t.Errorf("expected no overrides, got %+v", model.RetryPolicy)
	}

	attrs := model.EffectiveRetryPolicy.Attributes()
	if attrs["max_retries"].(types.Int64).ValueInt64() != 5 {
		t.Errorf("expected max_retries 5, got %v", attrs["max_retries"])
	}
	if attrs["timeout_ms"].(types.Int64).ValueInt64() != 30000 {
		t.Errorf("expected timeout_ms 30000, got %v", attrs["timeout_ms"])
	}
	if attrs["backoff_strategy"].(types.String).ValueString() != "exponential" {
		t.Errorf("expected exponential, got %v", attrs["backoff_strategy"])
	}
}

func baseModel() destinationModel {
	return destinationModel{
		Name:        types.StringValue("My Receiver"),
		URL:         types.StringValue("https://example.com/webhook"),
		Method:      types.StringValue("POST"),
		ContentType: types.StringValue("application/json"),
		TimeoutMs:   types.Int64Null(),
		IsEnabled:   types.BoolValue(true),
		Headers:     types.MapNull(types.StringType),
	}
}

func withTimeout(timeout types.Int64) destinationModel {
	m := baseModel()
	m.TimeoutMs = timeout
	return m
}

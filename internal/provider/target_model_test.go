package provider

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/uptimepage/terraform-provider-uptimepage/internal/client"
)

func strptr(s string) *string { return &s }

// TestCheckToModel_RedactionSuppressed is the load-bearing test: the API
// returns basic_auth / bearer_token redacted, and the mapper must keep the
// prior (real) state so there is no perpetual diff.
func TestCheckToModel_RedactionSuppressed(t *testing.T) {
	ctx := context.Background()
	prior := checkModel{Type: types.StringValue(client.CheckTypeHTTP), HTTP: &httpCheckModel{
		BasicAuth:   &basicAuthModel{Username: types.StringValue("user"), Password: types.StringValue("pass")},
		BearerToken: types.StringValue("real-token"),
	}}
	spec := client.CheckSpec{Type: client.CheckTypeHTTP, HTTP: &client.HTTPCheck{
		URL:            "https://example.com",
		Method:         "GET",
		Timeout:        5000,
		ExpectedStatus: client.ExpectedStatus{Kind: client.StatusKindExact, Exact: 200},
		Headers:        map[string]string{},
		BasicAuth:      &[2]string{redactedSentinel, redactedSentinel},
		BearerToken:    strptr(redactedSentinel),
	}}

	got, d := checkToModel(ctx, prior, spec)
	if d.HasError() {
		t.Fatalf("diags: %v", d)
	}
	if got.HTTP == nil {
		t.Fatal("http model nil")
	}
	if got.HTTP.BasicAuth == nil || got.HTTP.BasicAuth.Username.ValueString() != "user" || got.HTTP.BasicAuth.Password.ValueString() != "pass" {
		t.Errorf("basic_auth not preserved from prior: %+v", got.HTTP.BasicAuth)
	}
	if got.HTTP.BearerToken.ValueString() != "real-token" {
		t.Errorf("bearer_token not preserved: %q", got.HTTP.BearerToken.ValueString())
	}
}

// TestCheckToModel_ClearedSecretsReflected: when the API reports the secret as
// absent (not redacted), the model should reflect the cleared value.
func TestCheckToModel_ClearedSecretsReflected(t *testing.T) {
	ctx := context.Background()
	prior := checkModel{Type: types.StringValue(client.CheckTypeHTTP), HTTP: &httpCheckModel{
		BasicAuth:   &basicAuthModel{Username: types.StringValue("user"), Password: types.StringValue("pass")},
		BearerToken: types.StringValue("real-token"),
	}}
	spec := client.CheckSpec{Type: client.CheckTypeHTTP, HTTP: &client.HTTPCheck{
		URL: "https://example.com", Method: "GET", Timeout: 5000,
		ExpectedStatus: client.ExpectedStatus{Kind: client.StatusKindExact, Exact: 200},
		Headers:        map[string]string{},
		BasicAuth:      nil,
		BearerToken:    nil,
	}}

	got, d := checkToModel(ctx, prior, spec)
	if d.HasError() {
		t.Fatalf("diags: %v", d)
	}
	if got.HTTP == nil {
		t.Fatal("http model nil")
	}
	if got.HTTP.BasicAuth != nil {
		t.Errorf("basic_auth should be nil when API clears it, got %+v", got.HTTP.BasicAuth)
	}
	if !got.HTTP.BearerToken.IsNull() {
		t.Errorf("bearer_token should be null when API clears it, got %q", got.HTTP.BearerToken.ValueString())
	}
}

func TestExpectedStatus_ModelWireRoundTrip(t *testing.T) {
	ctx := context.Background()
	oneOf, d := types.ListValueFrom(ctx, types.Int64Type, []int64{200, 204})
	if d.HasError() {
		t.Fatalf("list build: %v", d)
	}

	cases := map[string]expectedStatusModel{
		"exact":  {Kind: types.StringValue(client.StatusKindExact), Exact: types.Int64Value(200), OneOf: types.ListNull(types.Int64Type)},
		"range":  {Kind: types.StringValue(client.StatusKindRange), Exact: types.Int64Null(), Range: &rangeModel{Min: types.Int64Value(200), Max: types.Int64Value(299)}, OneOf: types.ListNull(types.Int64Type)},
		"one_of": {Kind: types.StringValue(client.StatusKindOneOf), Exact: types.Int64Null(), OneOf: oneOf},
	}
	for name, in := range cases {
		t.Run(name, func(t *testing.T) {
			wire, d := in.toWire(ctx)
			if d.HasError() {
				t.Fatalf("toWire: %v", d)
			}
			back, d := expectedStatusToModel(ctx, wire)
			if d.HasError() {
				t.Fatalf("toModel: %v", d)
			}
			if back.Kind.ValueString() != in.Kind.ValueString() {
				t.Errorf("kind = %q, want %q", back.Kind.ValueString(), in.Kind.ValueString())
			}
			switch in.Kind.ValueString() {
			case client.StatusKindExact:
				if back.Exact.ValueInt64() != 200 {
					t.Errorf("exact = %d", back.Exact.ValueInt64())
				}
			case client.StatusKindRange:
				if back.Range == nil || back.Range.Min.ValueInt64() != 200 || back.Range.Max.ValueInt64() != 299 {
					t.Errorf("range = %+v", back.Range)
				}
			case client.StatusKindOneOf:
				var codes []int64
				back.OneOf.ElementsAs(ctx, &codes, false)
				if len(codes) != 2 || codes[0] != 200 || codes[1] != 204 {
					t.Errorf("one_of = %v", codes)
				}
			}
		})
	}
}

func TestExpectedStatus_KindPayloadMismatchErrors(t *testing.T) {
	ctx := context.Background()
	cases := map[string]expectedStatusModel{
		"exact missing exact": {Kind: types.StringValue(client.StatusKindExact), Exact: types.Int64Null(), OneOf: types.ListNull(types.Int64Type)},
		"range missing block": {Kind: types.StringValue(client.StatusKindRange), Exact: types.Int64Null(), OneOf: types.ListNull(types.Int64Type)},
		"one_of empty":        {Kind: types.StringValue(client.StatusKindOneOf), Exact: types.Int64Null(), OneOf: types.ListNull(types.Int64Type)},
	}
	for name, in := range cases {
		t.Run(name, func(t *testing.T) {
			_, d := in.toWire(ctx)
			if !d.HasError() {
				t.Errorf("expected a diagnostic error for %q, got none", name)
			}
		})
	}
}

func TestToNew_MapsCoreFields(t *testing.T) {
	ctx := context.Background()
	tags, _ := types.SetValueFrom(ctx, types.StringType, []string{"prod"})
	headers, _ := types.MapValueFrom(ctx, types.StringType, map[string]string{"X-A": "1"})

	m := targetModel{
		Name:         types.StringValue("api"),
		Interval:     types.Int64Value(60),
		Enabled:      types.BoolValue(true),
		Tags:         tags,
		PublicStatus: types.BoolValue(false),
		GroupName:    types.StringValue("group"),
		Check: checkModel{
			Type: types.StringValue(client.CheckTypeHTTP),
			HTTP: &httpCheckModel{
				URL:            types.StringValue("https://example.com"),
				Method:         types.StringValue("GET"),
				TimeoutMs:      types.Int64Value(5000),
				MaxRedirects:   types.Int64Value(5),
				ExpectedStatus: expectedStatusModel{Kind: types.StringValue(client.StatusKindExact), Exact: types.Int64Value(200), OneOf: types.ListNull(types.Int64Type)},
				Headers:        headers,
				VerifyTLS:      types.BoolValue(true),
				BearerToken:    types.StringNull(),
			},
		},
	}
	out, d := m.toNew(ctx)
	if d.HasError() {
		t.Fatalf("toNew: %v", d)
	}
	if out.Name != "api" || out.Interval != 60 || len(out.Tags) != 1 || out.Tags[0] != "prod" {
		t.Errorf("core fields wrong: %+v", out)
	}
	if out.GroupName == nil || *out.GroupName != "group" {
		t.Errorf("group_name = %v", out.GroupName)
	}
	if out.Check.HTTP == nil || out.Check.HTTP.Headers["X-A"] != "1" {
		t.Errorf("headers not mapped: %+v", out.Check.HTTP)
	}
}

func TestCheckToModel_TCPVariant(t *testing.T) {
	ctx := context.Background()
	spec := client.CheckSpec{Type: client.CheckTypeTCP, TCP: &client.TCPCheck{Host: "db", Port: 5432, Timeout: 3000}}
	got, d := checkToModel(ctx, checkModel{}, spec)
	if d.HasError() {
		t.Fatalf("diags: %v", d)
	}
	if got.TCP == nil || got.TCP.Host.ValueString() != "db" || got.TCP.Port.ValueInt64() != 5432 {
		t.Errorf("tcp not mapped: %+v", got.TCP)
	}
	if got.HTTP != nil {
		t.Error("http should be nil for a tcp check")
	}
}

func TestCheckToWire_DNS(t *testing.T) {
	ctx := context.Background()
	c := checkModel{Type: types.StringValue(client.CheckTypeDNS), DNS: &dnsCheckModel{
		Domain:     types.StringValue("x.com"),
		RecordType: types.StringValue("A"),
		Resolver:   types.StringValue("1.1.1.1"),
		TimeoutMs:  types.Int64Value(5000),
	}}
	out, d := c.toWire(ctx)
	if d.HasError() {
		t.Fatalf("toWire: %v", d)
	}
	if out.DNS == nil || out.DNS.RecordType != "A" || out.DNS.Resolver == nil || *out.DNS.Resolver != "1.1.1.1" {
		t.Errorf("dns wire wrong: %+v", out.DNS)
	}
}

func TestCheckToWire_MissingBlockErrors(t *testing.T) {
	ctx := context.Background()
	c := checkModel{Type: types.StringValue(client.CheckTypeTCP)} // no tcp block
	_, d := c.toWire(ctx)
	if !d.HasError() {
		t.Error("expected error when the block for the type is missing")
	}
}

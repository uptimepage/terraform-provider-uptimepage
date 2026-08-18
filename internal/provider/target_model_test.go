package provider

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/diag"
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
		Name:      types.StringValue("api"),
		Interval:  types.Int64Value(60),
		Enabled:   types.BoolValue(true),
		Tags:      tags,
		GroupName: types.StringValue("group"),
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

func TestRegions_ExtractAndRoundTrip(t *testing.T) {
	ctx := context.Background()
	var diags diag.Diagnostics

	// Null/unknown extract to nil ("leave the server-assigned set in place").
	if got := (targetModel{Regions: types.SetNull(types.StringType)}).regions(ctx, &diags); got != nil {
		t.Errorf("null regions = %v, want nil", got)
	}
	if got := (targetModel{Regions: types.SetUnknown(types.StringType)}).regions(ctx, &diags); got != nil {
		t.Errorf("unknown regions = %v, want nil", got)
	}

	// A configured set extracts to a slice, and a slice read back from the API
	// round-trips to an equal Set.
	set, d := types.SetValueFrom(ctx, types.StringType, []string{"us-east", "apac-sg"})
	diags.Append(d...)
	got := (targetModel{Regions: set}).regions(ctx, &diags)
	if len(got) != 2 {
		t.Fatalf("regions = %v, want 2 elements", got)
	}
	if back := regionsToSet(ctx, got, &diags); !back.Equal(set) {
		t.Errorf("round-trip set = %v, want %v", back, set)
	}
	if diags.HasError() {
		t.Fatalf("diags: %v", diags)
	}
}

func TestCheckPingVariantRoundTrips(t *testing.T) {
	ctx := context.Background()
	spec := client.CheckSpec{Type: client.CheckTypePing, Ping: &client.PingCheck{Host: "gateway.example.com", Timeout: 3000}}
	got, d := checkToModel(ctx, checkModel{}, spec)
	if d.HasError() {
		t.Fatalf("diags: %v", d)
	}
	if got.Ping == nil || got.Ping.Host.ValueString() != "gateway.example.com" || got.Ping.TimeoutMs.ValueInt64() != 3000 {
		t.Fatalf("ping not mapped: %+v", got.Ping)
	}
	// A ping is not a connect probe: leaking into the tcp block would send the
	// API a port it has no field for.
	if got.TCP != nil {
		t.Error("tcp should be nil for a ping check")
	}
	if got.Type.ValueString() != client.CheckTypePing {
		t.Errorf("discriminator = %q, want %q", got.Type.ValueString(), client.CheckTypePing)
	}
	back, d := got.toWire(ctx)
	if d.HasError() {
		t.Fatalf("diags: %v", d)
	}
	if back.Ping == nil || back.Ping.Host != "gateway.example.com" || back.Ping.Timeout != 3000 {
		t.Fatalf("round-trip lost the ping: %+v", back.Ping)
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

func TestCheckToWire_Flow(t *testing.T) {
	ctx := context.Background()
	c := checkModel{Type: types.StringValue(client.CheckTypeFlow), Flow: &flowCheckModel{
		StartURL: types.StringValue("https://app.example.com/login"),
		Steps: []flowStepModel{
			{Op: types.StringValue(client.FlowOpFill), URL: types.StringNull(), Selector: types.StringValue("#u"), Value: types.StringValue("secret"), Contains: types.StringNull()},
			{Op: types.StringValue(client.FlowOpAssertText), URL: types.StringNull(), Selector: types.StringNull(), Value: types.StringNull(), Contains: types.StringValue("Welcome")},
		},
		TimeoutMs:     types.Int64Value(30000),
		StepTimeoutMs: types.Int64Value(5000),
		VerifyTLS:     types.BoolValue(true),
	}}
	out, d := c.toWire(ctx)
	if d.HasError() {
		t.Fatalf("toWire: %v", d)
	}
	if out.Flow == nil || len(out.Flow.Steps) != 2 {
		t.Fatalf("flow wire wrong: %+v", out.Flow)
	}
	fill := out.Flow.Steps[0]
	if fill.Op != client.FlowOpFill || fill.Selector == nil || *fill.Selector != "#u" || fill.Value != "secret" {
		t.Errorf("fill step wrong: %+v", fill)
	}
	// assert_text with no selector expands to a nil selector (page-wide assertion).
	if out.Flow.Steps[1].Selector != nil {
		t.Errorf("assert_text selector = %q, want nil", *out.Flow.Steps[1].Selector)
	}
}

// TestFlowToModel_RedactionSuppressed pins the flow analog of the http secret
// carry: the API returns a fill value redacted, and the mapper keeps the prior
// (real) value at the same step index so there is no perpetual diff.
func TestFlowToModel_RedactionSuppressed(t *testing.T) {
	ctx := context.Background()
	prior := checkModel{Type: types.StringValue(client.CheckTypeFlow), Flow: &flowCheckModel{
		Steps: []flowStepModel{
			{Op: types.StringValue(client.FlowOpFill), Selector: types.StringValue("#p"), Value: types.StringValue("real-pass")},
		},
	}}
	sel := "#p"
	spec := client.CheckSpec{Type: client.CheckTypeFlow, Flow: &client.FlowCheck{
		StartURL: "https://app.example.com/login",
		Steps:    []client.FlowStep{{Op: client.FlowOpFill, Selector: &sel, Value: redactedSentinel}},
		Timeout:  30000, StepTimeout: 5000, VerifyTLS: true,
	}}

	got, d := checkToModel(ctx, prior, spec)
	if d.HasError() {
		t.Fatalf("diags: %v", d)
	}
	if got.Flow == nil || len(got.Flow.Steps) != 1 {
		t.Fatalf("flow model wrong: %+v", got.Flow)
	}
	if got.Flow.Steps[0].Value.ValueString() != "real-pass" {
		t.Errorf("fill value not preserved from prior: %q", got.Flow.Steps[0].Value.ValueString())
	}
}

func TestValidateFlowSteps(t *testing.T) {
	var ok diag.Diagnostics
	validateFlowSteps([]flowStepModel{
		{Op: types.StringValue(client.FlowOpGoto), URL: types.StringValue("https://x/login")},
		{Op: types.StringValue(client.FlowOpFill), Selector: types.StringValue("#u"), Value: types.StringValue("v")},
		{Op: types.StringValue(client.FlowOpAssertText), Contains: types.StringValue("Hi")},
	}, &ok)
	if ok.HasError() {
		t.Errorf("well-formed steps should validate, got %v", ok)
	}

	var bad diag.Diagnostics
	validateFlowSteps([]flowStepModel{
		{Op: types.StringValue(client.FlowOpGoto), URL: types.StringNull()},                                     // missing url
		{Op: types.StringValue(client.FlowOpFill), Selector: types.StringNull(), Value: types.StringValue("v")}, // missing selector
		{Op: types.StringValue(client.FlowOpAssertURL), Contains: types.StringValue("")},                        // empty contains
	}, &bad)
	if n := len(bad.Errors()); n != 3 {
		t.Errorf("expected 3 errors, got %d: %v", n, bad)
	}

	// An unknown value defers to apply-time rather than erroring at plan.
	var unk diag.Diagnostics
	validateFlowSteps([]flowStepModel{
		{Op: types.StringValue(client.FlowOpGoto), URL: types.StringUnknown()},
	}, &unk)
	if unk.HasError() {
		t.Errorf("unknown field should defer to apply-time, got %v", unk)
	}
}

// TestCheckToWire_WriteOnlySecrets: the write-only twins feed the wire payload
// when the in-state secrets are absent.
func TestCheckToWire_WriteOnlySecrets(t *testing.T) {
	ctx := context.Background()
	c := checkModel{Type: types.StringValue(client.CheckTypeHTTP), HTTP: &httpCheckModel{
		URL:            types.StringValue("https://example.com"),
		Method:         types.StringValue("GET"),
		TimeoutMs:      types.Int64Value(5000),
		ExpectedStatus: expectedStatusModel{Kind: types.StringValue(client.StatusKindExact), Exact: types.Int64Value(200)},
		BasicAuth: &basicAuthModel{
			Username:          types.StringValue("user"),
			PasswordWo:        types.StringValue("wo-pass"),
			PasswordWoVersion: types.Int64Value(1),
		},
		BearerTokenWo:        types.StringValue("wo-token"),
		BearerTokenWoVersion: types.Int64Value(1),
	}}
	out, d := c.toWire(ctx)
	if d.HasError() {
		t.Fatalf("toWire: %v", d)
	}
	if out.HTTP.BasicAuth == nil || out.HTTP.BasicAuth[0] != "user" || out.HTTP.BasicAuth[1] != "wo-pass" {
		t.Errorf("password_wo not sent: %+v", out.HTTP.BasicAuth)
	}
	if out.HTTP.BearerToken == nil || *out.HTTP.BearerToken != "wo-token" {
		t.Errorf("bearer_token_wo not sent: %v", out.HTTP.BearerToken)
	}
}

// TestCheckToModel_RotationVersionsSurviveRedaction: the version triggers are
// ordinary attrs the API never sees; the redacted read-back must not drop them
// or the applied state would diverge from the plan.
func TestCheckToModel_RotationVersionsSurviveRedaction(t *testing.T) {
	ctx := context.Background()
	prior := checkModel{Type: types.StringValue(client.CheckTypeHTTP), HTTP: &httpCheckModel{
		BasicAuth: &basicAuthModel{
			Username:          types.StringValue("user"),
			PasswordWoVersion: types.Int64Value(3),
		},
		BearerTokenWoVersion: types.Int64Value(2),
	}}
	spec := client.CheckSpec{Type: client.CheckTypeHTTP, HTTP: &client.HTTPCheck{
		URL: "https://example.com", Method: "GET", Timeout: 5000,
		ExpectedStatus: client.ExpectedStatus{Kind: client.StatusKindExact, Exact: 200},
		Headers:        map[string]string{},
		BasicAuth:      &[2]string{redactedSentinel, redactedSentinel},
		BearerToken:    strptr(redactedSentinel),
	}}
	got, d := checkToModel(ctx, prior, spec)
	if d.HasError() {
		t.Fatalf("diags: %v", d)
	}
	if got.HTTP.BasicAuth == nil || got.HTTP.BasicAuth.PasswordWoVersion.ValueInt64() != 3 {
		t.Errorf("password_wo_version dropped: %+v", got.HTTP.BasicAuth)
	}
	if got.HTTP.BearerTokenWoVersion.ValueInt64() != 2 {
		t.Errorf("bearer_token_wo_version dropped: %v", got.HTTP.BearerTokenWoVersion)
	}
}

func TestKeepBasicAuth_NonRedactedKeepsVersion(t *testing.T) {
	prior := &basicAuthModel{Username: types.StringValue("u"), PasswordWoVersion: types.Int64Value(4)}
	got := keepBasicAuth(prior, &[2]string{"u2", "p2"})
	if got == nil || got.Username.ValueString() != "u2" || got.Password.ValueString() != "p2" {
		t.Fatalf("api values not reflected: %+v", got)
	}
	if got.PasswordWoVersion.ValueInt64() != 4 {
		t.Errorf("version dropped: %+v", got)
	}
}

func TestGraftWriteOnlySecrets(t *testing.T) {
	plan := targetModel{Check: checkModel{Type: types.StringValue(client.CheckTypeHTTP), HTTP: &httpCheckModel{
		BasicAuth: &basicAuthModel{Username: types.StringValue("u")},
	}}}
	cfg := targetModel{Check: checkModel{Type: types.StringValue(client.CheckTypeHTTP), HTTP: &httpCheckModel{
		BasicAuth:     &basicAuthModel{Username: types.StringValue("u"), PasswordWo: types.StringValue("s3cret")},
		BearerTokenWo: types.StringValue("tok"),
	}}}
	graftWriteOnlySecrets(&plan, cfg)
	if plan.Check.HTTP.BasicAuth.PasswordWo.ValueString() != "s3cret" {
		t.Errorf("password_wo not grafted: %+v", plan.Check.HTTP.BasicAuth)
	}
	if plan.Check.HTTP.BearerTokenWo.ValueString() != "tok" {
		t.Errorf("bearer_token_wo not grafted: %v", plan.Check.HTTP.BearerTokenWo)
	}
}

// TestKeepBasicAuth_RedactedDropsWriteOnlyValue: the grafted write-only value
// must not ride the prior model back into state on the sentinel path.
func TestKeepBasicAuth_RedactedDropsWriteOnlyValue(t *testing.T) {
	prior := &basicAuthModel{
		Username:          types.StringValue("u"),
		PasswordWo:        types.StringValue("grafted-secret"),
		PasswordWoVersion: types.Int64Value(2),
	}
	got := keepBasicAuth(prior, &[2]string{redactedSentinel, redactedSentinel})
	if got == nil || got.Username.ValueString() != "u" || got.PasswordWoVersion.ValueInt64() != 2 {
		t.Fatalf("prior fields not kept: %+v", got)
	}
	if !got.PasswordWo.IsNull() {
		t.Errorf("write-only value persisted: %q", got.PasswordWo.ValueString())
	}
}

func TestGraftWriteOnlySecrets_BearerOnlyAndNilHTTP(t *testing.T) {
	plan := targetModel{Check: checkModel{Type: types.StringValue(client.CheckTypeHTTP), HTTP: &httpCheckModel{}}}
	cfg := targetModel{Check: checkModel{Type: types.StringValue(client.CheckTypeHTTP), HTTP: &httpCheckModel{
		BearerTokenWo: types.StringValue("tok"),
	}}}
	graftWriteOnlySecrets(&plan, cfg)
	if plan.Check.HTTP.BearerTokenWo.ValueString() != "tok" {
		t.Errorf("bearer_token_wo not grafted without basic_auth: %v", plan.Check.HTTP.BearerTokenWo)
	}

	tcpPlan := targetModel{Check: checkModel{Type: types.StringValue(client.CheckTypeTCP)}}
	graftWriteOnlySecrets(&tcpPlan, cfg) // must not panic on nil HTTP
	if tcpPlan.Check.HTTP != nil {
		t.Errorf("nil http block should stay nil: %+v", tcpPlan.Check.HTTP)
	}
}

// TestCheckToModel_NoPrior: the first read after import has no prior model;
// secrets land null and nothing panics.
func TestCheckToModel_NoPrior(t *testing.T) {
	ctx := context.Background()
	spec := client.CheckSpec{Type: client.CheckTypeHTTP, HTTP: &client.HTTPCheck{
		URL: "https://example.com", Method: "GET", Timeout: 5000,
		ExpectedStatus: client.ExpectedStatus{Kind: client.StatusKindExact, Exact: 200},
		Headers:        map[string]string{},
		BasicAuth:      &[2]string{redactedSentinel, redactedSentinel},
		BearerToken:    strptr(redactedSentinel),
	}}
	got, d := checkToModel(ctx, checkModel{}, spec)
	if d.HasError() {
		t.Fatalf("diags: %v", d)
	}
	if got.HTTP.BasicAuth != nil {
		t.Errorf("basic_auth should be null with no prior: %+v", got.HTTP.BasicAuth)
	}
	if !got.HTTP.BearerToken.IsNull() || !got.HTTP.BearerTokenWoVersion.IsNull() {
		t.Errorf("bearer fields should be null with no prior: %+v", got.HTTP)
	}
}

// A kind accepted by the schema but missing from the presence map is rejected
// at plan time with "requires the block" even when the block is set, so every
// config naming it fails. ping shipped that way until this caught it.
func TestEveryAcceptedCheckKindHasABlock(t *testing.T) {
	present := checkBlocksPresent(checkModel{})
	for _, kind := range checkKinds() {
		if _, ok := present[kind]; !ok {
			t.Errorf("check kind %q is accepted by the schema but has no block-presence entry", kind)
		}
	}
	if len(present) != len(checkKinds()) {
		t.Errorf("presence map has %d entries, schema accepts %d", len(present), len(checkKinds()))
	}
}

// The API parses start_url and every goto url and re-serialises them, so a URL
// written without a path reads back with a trailing slash. Echoing that into
// state fails Terraform's post-apply consistency check on a Required
// attribute, which is the failure http.url already avoids with keepURL.
func TestFlowToModel_KeepsTheConfiguredURLForm(t *testing.T) {
	ctx := context.Background()
	prior := checkModel{Type: types.StringValue(client.CheckTypeFlow), Flow: &flowCheckModel{
		StartURL: types.StringValue("https://app.example.com"),
		Steps: []flowStepModel{
			{Op: types.StringValue(client.FlowOpGoto), URL: types.StringValue("https://app.example.com/x")},
			{Op: types.StringValue(client.FlowOpGoto), URL: types.StringValue("https://other.example.com")},
		},
	}}
	spec := client.CheckSpec{Type: client.CheckTypeFlow, Flow: &client.FlowCheck{
		StartURL: "https://app.example.com/",
		Steps: []client.FlowStep{
			{Op: client.FlowOpGoto, URL: "https://app.example.com/x"},
			{Op: client.FlowOpGoto, URL: "https://other.example.com/"},
		},
		Timeout: 30000, StepTimeout: 5000, VerifyTLS: true,
	}}

	got, d := checkToModel(ctx, prior, spec)
	if d.HasError() {
		t.Fatalf("diags: %v", d)
	}
	if s := got.Flow.StartURL.ValueString(); s != "https://app.example.com" {
		t.Errorf("start_url = %q, want the configured form back", s)
	}
	if s := got.Flow.Steps[0].URL.ValueString(); s != "https://app.example.com/x" {
		t.Errorf("goto url with a path changed: %q", s)
	}
	if s := got.Flow.Steps[1].URL.ValueString(); s != "https://other.example.com" {
		t.Errorf("goto url = %q, want the configured form back", s)
	}
}

// Drift is not canonicalisation. A server URL that is a different address has
// to reach state, or the provider hides an out-of-band change.
func TestFlowToModel_RealDriftStillSurfaces(t *testing.T) {
	ctx := context.Background()
	prior := checkModel{Type: types.StringValue(client.CheckTypeFlow), Flow: &flowCheckModel{
		StartURL: types.StringValue("https://app.example.com"),
		Steps: []flowStepModel{
			{Op: types.StringValue(client.FlowOpGoto), URL: types.StringValue("https://app.example.com/x")},
		},
	}}
	spec := client.CheckSpec{Type: client.CheckTypeFlow, Flow: &client.FlowCheck{
		StartURL: "https://moved.example.com/",
		Steps:    []client.FlowStep{{Op: client.FlowOpGoto, URL: "https://app.example.com/y"}},
		Timeout:  30000, StepTimeout: 5000, VerifyTLS: true,
	}}

	got, d := checkToModel(ctx, prior, spec)
	if d.HasError() {
		t.Fatalf("diags: %v", d)
	}
	if s := got.Flow.StartURL.ValueString(); s != "https://moved.example.com/" {
		t.Errorf("start_url drift swallowed: %q", s)
	}
	if s := got.Flow.Steps[0].URL.ValueString(); s != "https://app.example.com/y" {
		t.Errorf("goto url drift swallowed: %q", s)
	}
}

// A create has no prior state, so the server value is all there is.
func TestFlowToModel_NoPriorTakesTheServerValue(t *testing.T) {
	ctx := context.Background()
	spec := client.CheckSpec{Type: client.CheckTypeFlow, Flow: &client.FlowCheck{
		StartURL: "https://app.example.com/",
		Steps:    []client.FlowStep{{Op: client.FlowOpGoto, URL: "https://app.example.com/x"}},
		Timeout:  30000, StepTimeout: 5000, VerifyTLS: true,
	}}
	got, d := checkToModel(ctx, checkModel{}, spec)
	if d.HasError() {
		t.Fatalf("diags: %v", d)
	}
	if s := got.Flow.StartURL.ValueString(); s != "https://app.example.com/" {
		t.Errorf("start_url = %q, want the server value", s)
	}
	if s := got.Flow.Steps[0].URL.ValueString(); s != "https://app.example.com/x" {
		t.Errorf("goto url = %q, want the server value", s)
	}
}

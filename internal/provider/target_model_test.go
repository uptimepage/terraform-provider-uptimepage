package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"

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

func TestToNew_CarriesFiringPolicyAndRegions(t *testing.T) {
	ctx := context.Background()
	regions, _ := types.SetValueFrom(ctx, types.StringType, []string{"eu-frankfurt"})

	m := targetModel{
		Name:                 types.StringValue("api"),
		Interval:             types.Int64Value(60),
		Enabled:              types.BoolValue(true),
		Tags:                 types.SetNull(types.StringType),
		Regions:              regions,
		Alerts:               []alertModel{{ChannelID: types.StringValue("c1")}},
		AlertConfirmations:   types.Int64Value(3),
		NotifyRecovery:       types.BoolValue(false),
		RenotifyIntervalSecs: types.Int64Value(0),
		RegionPolicy:         policyObject(ctx, t, client.RegionPolicyCount, types.Int64Value(2)),
		Check: checkModel{
			Type: types.StringValue(client.CheckTypeTCP),
			TCP:  &tcpCheckModel{Host: types.StringValue("db"), Port: types.Int64Value(5432), TimeoutMs: types.Int64Value(1000)},
		},
	}
	created, d := m.toNew(ctx)
	if d.HasError() {
		t.Fatalf("toNew: %v", d)
	}
	if created.AlertConfirmations != 3 || created.NotifyRecovery || created.RenotifyIntervalSecs != 0 {
		t.Errorf("firing policy not mapped on create: %+v", created)
	}
	if created.RegionPolicy == nil || *created.RegionPolicy != (client.RegionPolicy{Mode: client.RegionPolicyCount, Count: 2}) {
		t.Errorf("region_policy = %+v, want count 2", created.RegionPolicy)
	}
	if len(created.Alerts) != 1 || created.Alerts[0] != (client.AlertBinding{ChannelID: "c1"}) {
		t.Errorf("alerts = %+v, want the channel id alone", created.Alerts)
	}
	if len(created.Regions) != 1 || created.Regions[0] != "eu-frankfurt" {
		t.Errorf("regions = %v, want the configured set on the create body", created.Regions)
	}

	updated, d := m.toUpdate(ctx, m)
	if d.HasError() {
		t.Fatalf("toUpdate: %v", d)
	}
	if updated.AlertConfirmations != 3 || updated.NotifyRecovery || updated.RenotifyIntervalSecs != 0 {
		t.Errorf("firing policy not mapped on update: %+v", updated)
	}

	m.Regions = types.SetNull(types.StringType)
	for _, obj := range []types.Object{
		types.ObjectNull(regionPolicyObjectType.AttrTypes),
		types.ObjectUnknown(regionPolicyObjectType.AttrTypes),
	} {
		m.RegionPolicy = obj
		created, d = m.toNew(ctx)
		if d.HasError() {
			t.Fatalf("toNew: %v", d)
		}
		if created.Regions != nil {
			t.Errorf("omitted regions must stay off the create body, got %v", created.Regions)
		}
		if created.RegionPolicy != nil {
			t.Errorf("an absent region_policy must stay off the body, got %+v", created.RegionPolicy)
		}
	}
}

func policyObject(ctx context.Context, t *testing.T, mode string, count types.Int64) types.Object {
	t.Helper()
	obj, d := types.ObjectValueFrom(ctx, regionPolicyObjectType.AttrTypes, regionPolicyModel{
		Mode:  types.StringValue(mode),
		Count: count,
	})
	if d.HasError() {
		t.Fatalf("policy object: %v", d)
	}
	return obj
}

func TestTargetToModel_ReadsFiringPolicy(t *testing.T) {
	ctx := context.Background()
	got, d := targetToModel(ctx, targetModel{}, &client.Target{
		ID:       "t1",
		Name:     "api",
		Check:    client.CheckSpec{Type: client.CheckTypeTCP, TCP: &client.TCPCheck{Host: "db", Port: 5432, Timeout: 1000}},
		Interval: 60,
		Alerts:   []client.AlertBinding{{ChannelID: "c1"}},
		FiringPolicy: client.FiringPolicy{
			AlertConfirmations:   4,
			NotifyRecovery:       true,
			RenotifyIntervalSecs: 900,
			RegionPolicy:         &client.RegionPolicy{Mode: client.RegionPolicyAll},
		},
	})
	if d.HasError() {
		t.Fatalf("targetToModel: %v", d)
	}
	if got.AlertConfirmations.ValueInt64() != 4 || !got.NotifyRecovery.ValueBool() || got.RenotifyIntervalSecs.ValueInt64() != 900 {
		t.Errorf("firing policy not read back: %+v", got)
	}
	if !got.RegionPolicy.Equal(policyObject(ctx, t, client.RegionPolicyAll, types.Int64Null())) {
		t.Errorf("region_policy = %v, want mode all with a null count", got.RegionPolicy)
	}
	counted := regionPolicyToModel(ctx, &client.RegionPolicy{Mode: client.RegionPolicyCount, Count: 3}, &d)
	if !counted.Equal(policyObject(ctx, t, client.RegionPolicyCount, types.Int64Value(3))) {
		t.Errorf("count mode must carry its number, got %v", counted)
	}
	if len(got.Alerts) != 1 || got.Alerts[0].ChannelID.ValueString() != "c1" {
		t.Errorf("alerts = %+v", got.Alerts)
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

// The API requires warn_days > critical_days for both expiry kinds. Equal days
// is the config that reads as sensible and is not: the warning can never fire
// before the failure, so it planned clean and 400d at apply.
func TestValidateExpiryDays(t *testing.T) {
	at := path.Root("check").AtName("tls_cert")
	i := func(v int64) types.Int64 { return types.Int64Value(v) }

	for _, c := range []struct {
		warn, critical types.Int64
		wantErr        bool
		why            string
	}{
		{i(30), i(7), false, "warning comes first"},
		{i(2), i(1), false, "adjacent is still ordered"},
		{i(30), i(30), true, "equal, so the warning never fires"},
		{i(7), i(30), true, "inverted"},
		{types.Int64Unknown(), i(7), false, "unknown defers to apply"},
		{i(30), types.Int64Unknown(), false, "unknown defers to apply"},
		{types.Int64Null(), i(7), false, "null is the framework's Required check"},
	} {
		t.Run(c.why, func(t *testing.T) {
			var d diag.Diagnostics
			validateExpiryDays(at, c.warn, c.critical, &d)
			if got := d.HasError(); got != c.wantErr {
				t.Errorf("warn=%v critical=%v: error=%v, want %v (%v)", c.warn, c.critical, got, c.wantErr, d)
			}
		})
	}
}

// The error has to point at warn_days, not at the block, or a config with two
// expiry checks gives no clue which one is wrong.
func TestValidateExpiryDaysPointsAtTheAttribute(t *testing.T) {
	var d diag.Diagnostics
	validateExpiryDays(path.Root("check").AtName("domain_expiry"),
		types.Int64Value(10), types.Int64Value(10), &d)
	errs := d.Errors()
	if len(errs) != 1 {
		t.Fatalf("got %d errors, want 1", len(errs))
	}
	wp, ok := errs[0].(diag.DiagnosticWithPath)
	if !ok {
		t.Fatalf("diagnostic carries no path: %v", errs[0])
	}
	if got := wp.Path().String(); got != "check.domain_expiry.warn_days" {
		t.Errorf("error points at %q", got)
	}
}

// The API caps expiry days at 365 and rejects 0. The schema used to accept
// 0..36500, so anything outside the real range planned clean and 400d.
func TestExpiryDaysRangeMatchesTheAPI(t *testing.T) {
	var resp resource.SchemaResponse
	(&targetResource{}).Schema(context.Background(), resource.SchemaRequest{}, &resp)
	check := resp.Schema.Attributes["check"].(schema.SingleNestedAttribute)

	walked := 0
	for kind, attr := range check.Attributes {
		nested, ok := attr.(schema.SingleNestedAttribute)
		if !ok {
			continue
		}
		for name, a := range nested.Attributes {
			if name != "warn_days" && name != "critical_days" {
				continue
			}
			walked++
			ia := a.(schema.Int64Attribute)
			for _, c := range []struct {
				v       int64
				wantErr bool
			}{{0, true}, {1, false}, {365, false}, {366, true}, {36500, true}} {
				r := &validator.Int64Response{}
				for _, v := range ia.Validators {
					v.ValidateInt64(context.Background(), validator.Int64Request{
						Path:        path.Root("check").AtName(kind).AtName(name),
						ConfigValue: types.Int64Value(c.v),
					}, r)
				}
				if got := r.Diagnostics.HasError(); got != c.wantErr {
					t.Errorf("check.%s.%s = %d: error=%v, want %v", kind, name, c.v, got, c.wantErr)
				}
			}
		}
	}
	// tls_cert and domain_expiry, warn and critical each.
	if walked != 4 {
		t.Errorf("walked %d expiry-day attributes, want 4", walked)
	}
}

// The heartbeat block round-trips through the wire mapper, and max_runtime_ms
// keeps its absence: the API reads an absent max_runtime as "bounded by
// period+grace" and rejects a zero, so a null must not become 0.
func TestHeartbeatToWireAndBack(t *testing.T) {
	ctx := context.Background()
	for _, c := range []struct {
		maxRuntime types.Int64
		why        string
	}{
		{types.Int64Null(), "max_runtime omitted"},
		{types.Int64Value(900000), "max_runtime set"},
	} {
		t.Run(c.why, func(t *testing.T) {
			cm := checkModel{
				Type: types.StringValue(client.CheckTypeHeartbeat),
				Heartbeat: &heartbeatCheckModel{
					PeriodMs:     types.Int64Value(300000),
					GraceMs:      types.Int64Value(60000),
					MaxRuntimeMs: c.maxRuntime,
				},
			}
			spec, diags := cm.toWire(ctx)
			if diags.HasError() {
				t.Fatalf("toWire: %v", diags)
			}
			if spec.Heartbeat == nil {
				t.Fatal("heartbeat payload missing")
			}
			if spec.Heartbeat.Period != 300000 || spec.Heartbeat.Grace != 60000 {
				t.Errorf("period/grace wrong: %+v", spec.Heartbeat)
			}
			if (spec.Heartbeat.MaxRuntime == nil) != c.maxRuntime.IsNull() {
				t.Errorf("max_runtime presence wrong: %v", spec.Heartbeat.MaxRuntime)
			}

			back, d := checkToModel(ctx, checkModel{}, spec)
			if d.HasError() {
				t.Fatalf("checkToModel: %v", d)
			}
			if back.Heartbeat == nil {
				t.Fatal("heartbeat model missing on the way back")
			}
			if !back.Heartbeat.MaxRuntimeMs.Equal(c.maxRuntime) {
				t.Errorf("max_runtime_ms = %v, want %v", back.Heartbeat.MaxRuntimeMs, c.maxRuntime)
			}
		})
	}
}

// An interval coarser than period+grace steps over the deadline, and one under
// a minute is finer than evaluation runs. Both are API rejections today.
func TestValidateHeartbeatCadence(t *testing.T) {
	hb := func(period, grace int64) *heartbeatCheckModel {
		return &heartbeatCheckModel{
			PeriodMs: types.Int64Value(period),
			GraceMs:  types.Int64Value(grace),
		}
	}
	for _, c := range []struct {
		interval int64
		hb       *heartbeatCheckModel
		wantErr  bool
		why      string
	}{
		{60, hb(300000, 60000), false, "interval well inside the window"},
		{360, hb(300000, 60000), false, "interval exactly the window"},
		{361, hb(300000, 60000), true, "interval one second past the window"},
		{60, hb(60000, 0), false, "tightest window the API allows"},
		{59, hb(300000, 60000), true, "below the once-a-minute floor"},
	} {
		t.Run(c.why, func(t *testing.T) {
			var d diag.Diagnostics
			validateHeartbeatCadence(types.Int64Value(c.interval), c.hb, &d)
			if got := d.HasError(); got != c.wantErr {
				t.Errorf("interval=%d: error=%v, want %v (%v)", c.interval, got, c.wantErr, d)
			}
		})
	}

	// Unknown values resolve at apply, so flagging them fails plans that are fine.
	for _, c := range []struct {
		interval types.Int64
		hb       *heartbeatCheckModel
		why      string
	}{
		{types.Int64Unknown(), hb(300000, 60000), "unknown interval"},
		{types.Int64Value(60), &heartbeatCheckModel{PeriodMs: types.Int64Unknown(), GraceMs: types.Int64Value(0)}, "unknown period"},
		{types.Int64Value(60), &heartbeatCheckModel{PeriodMs: types.Int64Value(300000), GraceMs: types.Int64Unknown()}, "unknown grace"},
	} {
		t.Run(c.why, func(t *testing.T) {
			var d diag.Diagnostics
			validateHeartbeatCadence(c.interval, c.hb, &d)
			if d.HasError() {
				t.Errorf("%s should defer to apply: %v", c.why, d)
			}
		})
	}
}

// checkKinds drives the type validator, so a kind listed there without a
// matching nested attribute is accepted by `type` and then rejected as an
// unsupported argument. The presence-map guard above cannot see that.
func TestEveryAcceptedCheckKindHasASchemaBlock(t *testing.T) {
	var resp resource.SchemaResponse
	(&targetResource{}).Schema(context.Background(), resource.SchemaRequest{}, &resp)
	check := resp.Schema.Attributes["check"].(schema.SingleNestedAttribute)
	for _, kind := range checkKinds() {
		attr, ok := check.Attributes[kind]
		if !ok {
			t.Errorf("check kind %q is accepted by the type validator but has no %q block", kind, kind)
			continue
		}
		if _, ok := attr.(schema.SingleNestedAttribute); !ok {
			t.Errorf("check.%s is not a nested block", kind)
		}
	}
}

// Checked before the check kind is looked at, so a kind still unknown at plan
// does not skip it; a wholly unknown block defers to apply rather than failing
// the config read.
func TestRegionPolicyCountAtPlanTime(t *testing.T) {
	ctx := context.Background()
	obj := func(mode string, count types.Int64) types.Object {
		return policyObject(ctx, t, mode, count)
	}
	for _, c := range []struct {
		policy  types.Object
		wantErr bool
		why     string
	}{
		{types.ObjectNull(regionPolicyObjectType.AttrTypes), false, "omitted takes the default"},
		{types.ObjectUnknown(regionPolicyObjectType.AttrTypes), false, "unknown block defers to apply"},
		{obj("majority", types.Int64Null()), false, "a unit mode"},
		{obj("count", types.Int64Value(2)), false, "count with its number"},
		{obj("count", types.Int64Null()), true, "count without a number"},
		{obj("any", types.Int64Value(2)), true, "a number on a unit mode"},
	} {
		t.Run(c.why, func(t *testing.T) {
			cfg := targetModel{
				Name:         types.StringValue("x"),
				Interval:     types.Int64Value(60),
				Regions:      types.SetNull(types.StringType),
				RegionPolicy: c.policy,
				Check:        checkModel{Type: types.StringUnknown()},
			}
			var d diag.Diagnostics
			validateTargetConfig(ctx, cfg, &d)
			if got := d.HasError(); got != c.wantErr {
				t.Errorf("error=%v, want %v (%v)", got, c.wantErr, d)
			}
		})
	}
}

// A heartbeat is never probed, so the API refuses a create that names regions
// for one; the plan says so first and points at the attribute.
func TestHeartbeatRejectsRegionsAtPlanTime(t *testing.T) {
	set := func(vals ...string) types.Set {
		elems := make([]attr.Value, len(vals))
		for i, v := range vals {
			elems[i] = types.StringValue(v)
		}
		return types.SetValueMust(types.StringType, elems)
	}
	heartbeat := checkModel{
		Type: types.StringValue(client.CheckTypeHeartbeat),
		Heartbeat: &heartbeatCheckModel{
			PeriodMs: types.Int64Value(300000),
			GraceMs:  types.Int64Value(60000),
		},
	}

	for _, c := range []struct {
		check   checkModel
		regions types.Set
		wantErr bool
		why     string
	}{
		{heartbeat, set(), true, "an explicitly empty region set on a heartbeat"},
		{heartbeat, set("eu-helsinki"), true, "one region on a heartbeat"},
		{heartbeat, types.SetNull(types.StringType), false, "regions omitted"},
		{heartbeat, types.SetUnknown(types.StringType), false, "unknown defers to apply"},
		{
			checkModel{Type: types.StringValue(client.CheckTypeTCP), TCP: &tcpCheckModel{
				Host: types.StringValue("db"), Port: types.Int64Value(5432), TimeoutMs: types.Int64Value(3000),
			}},
			set("eu-helsinki"), false, "regions on a probed kind are fine",
		},
	} {
		t.Run(c.why, func(t *testing.T) {
			cfg := targetModel{
				Name:     types.StringValue("x"),
				Interval: types.Int64Value(60),
				Check:    c.check,
				Regions:  c.regions,
			}
			var d diag.Diagnostics
			validateTargetConfig(context.Background(), cfg, &d)
			if got := d.HasError(); got != c.wantErr {
				t.Errorf("error=%v, want %v (%v)", got, c.wantErr, d)
			}
		})
	}
}

// The server names the token's user as owner when a create omits the field.
// Optional alone made that a failed apply ("was null, but now ..."); computed
// with UseStateForUnknown absorbs the default and keeps it across updates.
func TestOwnerAbsorbsTheServerDefault(t *testing.T) {
	ctx := context.Background()
	var resp resource.SchemaResponse
	(&targetResource{}).Schema(ctx, resource.SchemaRequest{}, &resp)
	owner := resp.Schema.Attributes["owner_user_id"].(schema.StringAttribute)
	if !owner.Optional || !owner.Computed {
		t.Fatalf("owner_user_id must be optional and computed, got optional=%v computed=%v", owner.Optional, owner.Computed)
	}
	objType := resp.Schema.Type().TerraformType(ctx).(tftypes.Object)
	planOwner := func(state tfsdk.State, prior types.String) types.String {
		req := planmodifier.StringRequest{
			Path:       path.Root("owner_user_id"),
			State:      state,
			StateValue: prior,
			PlanValue:  types.StringUnknown(),
		}
		out := planmodifier.StringResponse{PlanValue: req.PlanValue}
		for _, m := range owner.PlanModifiers {
			m.PlanModifyString(ctx, req, &out)
		}
		return out.PlanValue
	}
	existing := tfsdk.State{Raw: rawWith(objType, "id", tftypes.NewValue(tftypes.String, "t1")), Schema: resp.Schema}

	if got := planOwner(tfsdk.State{Raw: tftypes.NewValue(objType, nil), Schema: resp.Schema}, types.StringNull()); !got.IsUnknown() {
		t.Errorf("a create leaves the owner to the server, got %v", got)
	}
	prior := types.StringValue("6f1a2b3c-0000-4000-8000-000000000001")
	if got := planOwner(existing, prior); !got.Equal(prior) {
		t.Errorf("an update that leaves the owner out should keep %v, got %v", prior, got)
	}
	if got := planOwner(existing, types.StringNull()); !got.IsNull() {
		t.Errorf("a target the server left unowned stays null in the plan, got %v", got)
	}
}

// An unknown owner must leave the create body: a null would read as "unowned"
// and skip the server default the schema promises.
func TestOwnerUnknownIsAbsentFromTheCreateBody(t *testing.T) {
	ctx := context.Background()
	m := targetModel{
		Name:        types.StringValue("db"),
		Interval:    types.Int64Value(60),
		Enabled:     types.BoolValue(true),
		OwnerUserID: types.StringUnknown(),
		Check: checkModel{
			Type: types.StringValue(client.CheckTypeTCP),
			TCP:  &tcpCheckModel{Host: types.StringValue("db"), Port: types.Int64Value(5432), TimeoutMs: types.Int64Value(1000)},
		},
	}
	created, d := m.toNew(ctx)
	if d.HasError() {
		t.Fatalf("toNew: %v", d)
	}
	raw, err := json.Marshal(created)
	if err != nil {
		t.Fatal(err)
	}
	var body map[string]json.RawMessage
	if err := json.Unmarshal(raw, &body); err != nil {
		t.Fatal(err)
	}
	if _, present := body["owner_user_id"]; present {
		t.Errorf("create body carries owner_user_id: %s", raw)
	}
}

// The PATCH body carries the owner only when the config names one; the plan's
// copy of the state must not be written back.
func TestOwnerOnUpdateComesFromTheConfig(t *testing.T) {
	ctx := context.Background()
	plan := targetModel{
		Name:        types.StringValue("db"),
		Interval:    types.Int64Value(60),
		Enabled:     types.BoolValue(true),
		OwnerUserID: types.StringValue("6f1a2b3c-0000-4000-8000-000000000001"),
		Check: checkModel{
			Type: types.StringValue(client.CheckTypeTCP),
			TCP:  &tcpCheckModel{Host: types.StringValue("db"), Port: types.Int64Value(5432), TimeoutMs: types.Int64Value(1000)},
		},
	}
	cfg := plan
	cfg.OwnerUserID = types.StringNull()

	unmanaged, d := plan.toUpdate(ctx, cfg)
	if d.HasError() {
		t.Fatalf("toUpdate: %v", d)
	}
	raw, _ := json.Marshal(unmanaged)
	if unmanaged.OwnerUserID != nil || bytes.Contains(raw, []byte("owner_user_id")) {
		t.Errorf("owner left out of the config still travels on PATCH: %s", raw)
	}

	managed, d := plan.toUpdate(ctx, plan)
	if d.HasError() {
		t.Fatalf("toUpdate: %v", d)
	}
	if managed.OwnerUserID == nil || *managed.OwnerUserID != plan.OwnerUserID.ValueString() {
		t.Errorf("configured owner dropped from PATCH: %+v", managed.OwnerUserID)
	}
}

func TestUUIDValidatorRefusesWhatTheAPIWouldRespell(t *testing.T) {
	for _, c := range []struct {
		v  string
		ok bool
	}{
		{"6f1a2b3c-0000-4000-8000-000000000001", true},
		{"6F1A2B3C-0000-4000-8000-000000000001", false},
		{"6f1a2b3c000040008000000000000001", false},
		{"not-a-uuid", false},
	} {
		r := &validator.StringResponse{}
		uuidValidator().ValidateString(context.Background(), validator.StringRequest{
			Path:        path.Root("owner_user_id"),
			ConfigValue: types.StringValue(c.v),
		}, r)
		if r.Diagnostics.HasError() == c.ok {
			t.Errorf("%q: ok=%v, diagnostics=%v", c.v, c.ok, r.Diagnostics)
		}
	}
}

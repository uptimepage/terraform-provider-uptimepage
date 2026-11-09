package provider

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"

	"github.com/uptimepage/terraform-provider-uptimepage/internal/client"
)

const (
	sourceUUID  = "019a6b2c-3d4e-7f80-9a1b-2c3d4e5f6a7b"
	policyUUID  = "019a6b2c-3d4e-7f80-9a1b-2c3d4e5f6a7c"
	channelUUID = "019a6b2c-3d4e-7f80-9a1b-2c3d4e5f6a7d"
)

func sourcePlan() alertSourceModel {
	return alertSourceModel{
		ID:                     types.StringValue(sourceUUID),
		Name:                   types.StringValue("prod alertmanager"),
		Kind:                   types.StringValue(client.AlertSourceKindAlertmanager),
		Enabled:                types.BoolValue(true),
		Grouping:               types.StringValue(client.AlertGroupingGroup),
		EscalationPolicyID:     types.StringValue(policyUUID),
		Alerts:                 []alertModel{{ChannelID: types.StringValue(channelUUID)}},
		NotifyRecovery:         types.BoolValue(true),
		RenotifyIntervalSecs:   types.Int64Value(3600),
		SigningSecret:          types.StringNull(),
		SigningSecretWo:        types.StringNull(),
		SigningSecretWoVersion: types.Int64Null(),
		IngestURL:              types.StringNull(),
		IngestToken:            types.StringNull(),
	}
}

func wireKeys(t *testing.T, u client.AlertSourceUpdate) map[string]json.RawMessage {
	t.Helper()
	buf, err := json.Marshal(u)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var out map[string]json.RawMessage
	if err := json.Unmarshal(buf, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	return out
}

// A rename must not resend the route: the API checks a route it is sent, and
// a source whose route was lost elsewhere would refuse the rename.
func TestAlertSourceUpdate_LeavesAnUnchangedRouteOut(t *testing.T) {
	prior := sourcePlan()
	plan := sourcePlan()
	plan.Name = types.StringValue("prod alertmanager eu")
	body := wireKeys(t, plan.toUpdate(prior))
	for _, k := range []string{"alerts", "escalation_policy_id", "signing_secret"} {
		if _, ok := body[k]; ok {
			t.Errorf("%s sent on a rename: %s", k, body[k])
		}
	}
	for _, k := range []string{"name", "enabled", "grouping", "notify_recovery", "renotify_interval_secs"} {
		if _, ok := body[k]; !ok {
			t.Errorf("%s missing from the PATCH", k)
		}
	}
}

func TestAlertSourceUpdate_SendsAChangedRoute(t *testing.T) {
	prior := sourcePlan()
	plan := sourcePlan()
	plan.Alerts = []alertModel{}
	plan.EscalationPolicyID = types.StringValue(channelUUID)
	body := wireKeys(t, plan.toUpdate(prior))
	if got := string(body["alerts"]); got != "[]" {
		t.Errorf("alerts = %s, want []", got)
	}
	if got := string(body["escalation_policy_id"]); got != `"`+channelUUID+`"` {
		t.Errorf("escalation_policy_id = %s", got)
	}
}

func TestAlertSourceUpdate_SendsTheWriteOnlySecretOnAVersionBump(t *testing.T) {
	prior := sourcePlan()
	prior.SigningSecretWoVersion = types.Int64Value(1)
	plan := prior
	plan.SigningSecretWo = types.StringValue("0123456789abcdef")
	if _, ok := wireKeys(t, plan.toUpdate(prior))["signing_secret"]; ok {
		t.Error("secret resent with an unchanged version")
	}
	plan.SigningSecretWoVersion = types.Int64Value(2)
	if got := string(wireKeys(t, plan.toUpdate(prior))["signing_secret"]); got != `"0123456789abcdef"` {
		t.Errorf("signing_secret = %s", got)
	}
}

// The API never returns the secret. An apply keeps the configured one; a
// refresh that finds the source unsigned (the secret was removed in the app)
// forgets it, so the next plan sends it again.
func TestAlertSourceSecret_OnlyARefreshForgetsIt(t *testing.T) {
	prior := sourcePlan()
	prior.Kind = types.StringValue(client.AlertSourceKindGrafana)
	prior.SigningSecret = types.StringValue("0123456789abcdef")
	prior.SigningSecretWoVersion = types.Int64Value(3)
	read := &client.AlertSource{ID: sourceUUID, Name: "prod grafana", Kind: client.AlertSourceKindGrafana, Grouping: client.AlertGroupingGroup}

	got := alertSourceToModel(prior, read)
	if !got.SigningSecret.Equal(prior.SigningSecret) || !got.SigningSecretWoVersion.Equal(prior.SigningSecretWoVersion) {
		t.Errorf("after apply: secret %v version %v, want the planned ones", got.SigningSecret, got.SigningSecretWoVersion)
	}
	if !got.SigningSecretWo.IsNull() {
		t.Error("a write-only value reached state")
	}
	got.forgetUnsignedSecret(read)
	if !got.SigningSecret.IsNull() || !got.SigningSecretWoVersion.IsNull() {
		t.Errorf("refresh, unsigned: secret %v version %v, want null", got.SigningSecret, got.SigningSecretWoVersion)
	}
	got = alertSourceToModel(prior, read)
	read.Signed = true
	got.forgetUnsignedSecret(read)
	if !got.SigningSecret.Equal(prior.SigningSecret) {
		t.Errorf("refresh, signed: secret %v, want kept", got.SigningSecret)
	}
}

// A read-scoped token, or a server whose key was rotated, reads the URL back
// null although the token still works.
func TestAlertSourceToModel_KeepsTheURLWhenTheAPIWithholdsIt(t *testing.T) {
	prior := sourcePlan()
	prior.IngestURL = types.StringValue("https://app.uptimepage.dev/hooks/alerts/tok")
	prior.IngestToken = types.StringValue("tok")
	got := alertSourceToModel(prior, &client.AlertSource{ID: sourceUUID, Name: "prod alertmanager", Kind: client.AlertSourceKindAlertmanager})
	if !got.IngestURL.Equal(prior.IngestURL) || !got.IngestToken.Equal(prior.IngestToken) {
		t.Errorf("url %v token %v, want the prior ones", got.IngestURL, got.IngestToken)
	}
}

func TestAlertSourceToModel_ReadsTheRouteAndURL(t *testing.T) {
	url := "https://app.uptimepage.dev/hooks/alerts/tok"
	read := &client.AlertSource{
		ID: sourceUUID, Name: "prod alertmanager", Kind: client.AlertSourceKindAlertmanager, Grouping: client.AlertGroupingAlert,
		EscalationPolicyID: strptr(policyUUID), IngestURL: &url, IngestToken: strptr("tok"),
	}
	got := alertSourceToModel(alertSourceModel{}, read)
	if got.Name.ValueString() != "prod alertmanager" || got.Grouping.ValueString() != client.AlertGroupingAlert {
		t.Errorf("name %v grouping %v", got.Name, got.Grouping)
	}
	if got.EscalationPolicyID.ValueString() != policyUUID || got.IngestURL.ValueString() != url || got.IngestToken.ValueString() != "tok" {
		t.Errorf("policy %v url %v token %v", got.EscalationPolicyID, got.IngestURL, got.IngestToken)
	}
	if got.Alerts == nil {
		t.Error("alerts read back nil, which diffs against the empty default")
	}
}

type fakeAlertSourceAPI struct {
	created   *client.AlertSource
	updates   []client.AlertSourceUpdate
	updateErr error
	deletes   []string
	deleteErr error
}

func (f *fakeAlertSourceAPI) CreateAlertSource(context.Context, client.NewAlertSource) (*client.AlertSource, error) {
	return f.created, nil
}

func (f *fakeAlertSourceAPI) GetAlertSource(context.Context, string) (*client.AlertSource, error) {
	return nil, errors.New("unused")
}

func (f *fakeAlertSourceAPI) UpdateAlertSource(_ context.Context, id string, in client.AlertSourceUpdate) (*client.AlertSource, error) {
	f.updates = append(f.updates, in)
	if f.updateErr != nil {
		return nil, f.updateErr
	}
	out := *f.created
	if in.Enabled != nil {
		out.Enabled = *in.Enabled
	}
	return &out, nil
}

func (f *fakeAlertSourceAPI) DeleteAlertSource(_ context.Context, id string) error {
	f.deletes = append(f.deletes, id)
	return f.deleteErr
}

func createdSource() *client.AlertSource {
	return &client.AlertSource{ID: sourceUUID, Name: "prod", Kind: client.AlertSourceKindAlertmanager, Enabled: true, Grouping: client.AlertGroupingGroup, NotifyRecovery: true, RenotifyIntervalSecs: 3600}
}

// A create cannot pause, so a source planned off is paused right after, with
// a PATCH that carries nothing else.
func TestAlertSourceCreate_PausesASourcePlannedOff(t *testing.T) {
	plan := sourcePlan()
	api := &fakeAlertSourceAPI{created: createdSource()}
	got, err := pauseIfDisabled(context.Background(), api, plan, api.created)
	if err != nil || got != api.created || len(api.updates) != 0 {
		t.Fatalf("enabled: got %v err %v updates %d", got, err, len(api.updates))
	}

	plan.Enabled = types.BoolValue(false)
	got, err = pauseIfDisabled(context.Background(), api, plan, api.created)
	if err != nil || got.Enabled || len(api.updates) != 1 {
		t.Fatalf("disabled: got %+v err %v updates %d", got, err, len(api.updates))
	}
	if body := wireKeys(t, api.updates[0]); len(body) != 1 || string(body["enabled"]) != "false" {
		t.Errorf("pause body = %v, want enabled alone", body)
	}
}

func createRequest(t *testing.T, enabled bool) (resource.Resource, resource.CreateRequest, *resource.CreateResponse) {
	t.Helper()
	r := newAlertSourceResource()
	sch, objType := resourceSchema(t, r)
	attrs := nullAttrs(objType)
	attrs["name"] = tftypes.NewValue(tftypes.String, "prod")
	attrs["kind"] = tftypes.NewValue(tftypes.String, client.AlertSourceKindAlertmanager)
	attrs["enabled"] = tftypes.NewValue(tftypes.Bool, enabled)
	attrs["grouping"] = tftypes.NewValue(tftypes.String, client.AlertGroupingGroup)
	attrs["alerts"] = tftypes.NewValue(objType.AttributeTypes["alerts"], []tftypes.Value{})
	attrs["notify_recovery"] = tftypes.NewValue(tftypes.Bool, true)
	attrs["renotify_interval_secs"] = tftypes.NewValue(tftypes.Number, 3600)
	cfg := tftypes.NewValue(objType, attrs)
	for _, computed := range []string{"id", "escalation_policy_id", "ingest_url", "ingest_token"} {
		attrs[computed] = tftypes.NewValue(tftypes.String, tftypes.UnknownValue)
	}
	req := resource.CreateRequest{
		Plan:   tfsdk.Plan{Schema: sch, Raw: tftypes.NewValue(objType, attrs)},
		Config: tfsdk.Config{Schema: sch, Raw: cfg},
	}
	resp := &resource.CreateResponse{State: tfsdk.State{Schema: sch, Raw: tftypes.NewValue(objType, nil)}}
	return r, req, resp
}

// A failed pause must not leave a listening source behind: the state would
// say enabled where the plan said off, which Terraform refuses.
func TestAlertSourceCreate_RemovesASourceItCouldNotPause(t *testing.T) {
	r, req, resp := createRequest(t, false)
	api := &fakeAlertSourceAPI{created: createdSource(), updateErr: errors.New("boom")}
	r.(*alertSourceResource).api = api
	r.Create(context.Background(), req, resp)
	if !resp.Diagnostics.HasError() || len(api.deletes) != 1 || api.deletes[0] != sourceUUID {
		t.Fatalf("diags %v deletes %v, want an error and the source removed", resp.Diagnostics, api.deletes)
	}
	if !resp.State.Raw.IsNull() {
		t.Error("state saved for a removed source")
	}

	r, req, resp = createRequest(t, false)
	api = &fakeAlertSourceAPI{created: createdSource(), updateErr: errors.New("boom"), deleteErr: errors.New("gone away")}
	r.(*alertSourceResource).api = api
	r.Create(context.Background(), req, resp)
	if resp.Diagnostics.ErrorsCount() != 2 || resp.State.Raw.IsNull() {
		t.Errorf("delete failed too: diags %v, state null %v; want both errors and the source tracked", resp.Diagnostics, resp.State.Raw.IsNull())
	}
}

func TestAlertSourceCreate_SavesAPausedSource(t *testing.T) {
	r, req, resp := createRequest(t, false)
	api := &fakeAlertSourceAPI{created: createdSource()}
	r.(*alertSourceResource).api = api
	r.Create(context.Background(), req, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("diags %v", resp.Diagnostics)
	}
	var got alertSourceModel
	resp.Diagnostics.Append(resp.State.Get(context.Background(), &got)...)
	if got.Enabled.ValueBool() || len(api.deletes) != 0 {
		t.Errorf("enabled %v deletes %v", got.Enabled, api.deletes)
	}
}

func alertSourceConfig(objType tftypes.Object, kind string, extra map[string]tftypes.Value) tftypes.Value {
	attrs := nullAttrs(objType)
	attrs["name"] = tftypes.NewValue(tftypes.String, "prod")
	attrs["kind"] = tftypes.NewValue(tftypes.String, kind)
	for k, v := range extra {
		attrs[k] = v
	}
	return tftypes.NewValue(objType, attrs)
}

func TestAlertSourceConfig_OnlyGrafanaTakesASecret(t *testing.T) {
	_, objType := resourceSchema(t, newAlertSourceResource())
	secret := map[string]tftypes.Value{"signing_secret": tftypes.NewValue(tftypes.String, "0123456789abcdef")}

	if diags := validateResourceConfig(t, "uptimepage_alert_source", objType, alertSourceConfig(objType, client.AlertSourceKindGrafana, secret)); hasError(diags) {
		t.Errorf("grafana: unexpected %v", diags)
	}
	if diags := validateResourceConfig(t, "uptimepage_alert_source", objType, alertSourceConfig(objType, client.AlertSourceKindDatadog, secret)); !hasErrorContaining(diags, "does not sign") {
		t.Errorf("datadog: want a refusal, got %v", diags)
	}
	short := map[string]tftypes.Value{"signing_secret": tftypes.NewValue(tftypes.String, "short")}
	if diags := validateResourceConfig(t, "uptimepage_alert_source", objType, alertSourceConfig(objType, client.AlertSourceKindGrafana, short)); !hasError(diags) {
		t.Error("a 5 character secret passed")
	}
	// Only kind and the secrets are read, so a route not yet known still plans.
	alertList := objType.AttributeTypes["alerts"]
	pending := map[string]tftypes.Value{"alerts": tftypes.NewValue(alertList, tftypes.UnknownValue)}
	if diags := validateResourceConfig(t, "uptimepage_alert_source", objType, alertSourceConfig(objType, client.AlertSourceKindDatadog, pending)); hasError(diags) {
		t.Errorf("unknown alerts: unexpected %v", diags)
	}
}

func TestAlertSourceKind_ChangeReplaces(t *testing.T) {
	sch, objType := resourceSchema(t, newAlertSourceResource())
	from := tftypes.NewValue(tftypes.String, client.AlertSourceKindAlertmanager)
	to := tftypes.NewValue(tftypes.String, client.AlertSourceKindGrafana)
	var req planmodifier.StringRequest
	req.State.Raw = rawWith(objType, "kind", from)
	req.Plan.Raw = rawWith(objType, "kind", to)
	req.StateValue = types.StringValue(client.AlertSourceKindAlertmanager)
	req.PlanValue = types.StringValue(client.AlertSourceKindGrafana)
	req.ConfigValue = req.PlanValue
	if !planString(t, sch, path.Root("kind"), req).RequiresReplace {
		t.Error("a kind change planned in place")
	}
}

package provider

import (
	"slices"

	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/uptimepage/terraform-provider-uptimepage/internal/client"
)

// alertSourceModel is the tfsdk view of an uptimepage_alert_source.
type alertSourceModel struct {
	ID                     types.String `tfsdk:"id"`
	Name                   types.String `tfsdk:"name"`
	Kind                   types.String `tfsdk:"kind"`
	Enabled                types.Bool   `tfsdk:"enabled"`
	Grouping               types.String `tfsdk:"grouping"`
	EscalationPolicyID     types.String `tfsdk:"escalation_policy_id"`
	Alerts                 []alertModel `tfsdk:"alerts"`
	NotifyRecovery         types.Bool   `tfsdk:"notify_recovery"`
	RenotifyIntervalSecs   types.Int64  `tfsdk:"renotify_interval_secs"`
	SigningSecret          types.String `tfsdk:"signing_secret"`
	SigningSecretWo        types.String `tfsdk:"signing_secret_wo"`
	SigningSecretWoVersion types.Int64  `tfsdk:"signing_secret_wo_version"`
	IngestURL              types.String `tfsdk:"ingest_url"`
	IngestToken            types.String `tfsdk:"ingest_token"`
}

func (m alertSourceModel) toNew() client.NewAlertSource {
	return client.NewAlertSource{
		Name:                 m.Name.ValueString(),
		Kind:                 m.Kind.ValueString(),
		Grouping:             m.Grouping.ValueString(),
		EscalationPolicyID:   optString(m.EscalationPolicyID),
		Alerts:               alertsToWire(m.Alerts),
		NotifyRecovery:       m.NotifyRecovery.ValueBool(),
		RenotifyIntervalSecs: uint32(m.RenotifyIntervalSecs.ValueInt64()),
		SigningSecret:        optString(firstSet(m.SigningSecret, m.SigningSecretWo)),
	}
}

// toUpdate is the PATCH from prior state to plan. The route goes only when it
// changed; a secret goes only when its value or its version did.
func (m alertSourceModel) toUpdate(prior alertSourceModel) client.AlertSourceUpdate {
	out := client.AlertSourceUpdate{
		Name:                 new(m.Name.ValueString()),
		Enabled:              new(m.Enabled.ValueBool()),
		Grouping:             new(m.Grouping.ValueString()),
		NotifyRecovery:       new(m.NotifyRecovery.ValueBool()),
		RenotifyIntervalSecs: new(uint32(m.RenotifyIntervalSecs.ValueInt64())),
	}
	if alerts := alertsToWire(m.Alerts); !slices.Equal(alerts, alertsToWire(prior.Alerts)) {
		out.Alerts = &alerts
	}
	// The plan keeps the prior binding when the line is left out, so a change
	// is always to another policy.
	if !m.EscalationPolicyID.Equal(prior.EscalationPolicyID) {
		out.EscalationPolicyID = optString(m.EscalationPolicyID)
	}
	switch {
	case !m.SigningSecret.IsNull() && !m.SigningSecret.Equal(prior.SigningSecret):
		out.SigningSecret = optString(m.SigningSecret)
	case !m.SigningSecretWo.IsNull() && !m.SigningSecretWoVersion.Equal(prior.SigningSecretWoVersion):
		out.SigningSecret = optString(m.SigningSecretWo)
	}
	return out
}

// alertSourceToModel maps a source the API returned into the model. The API
// never returns the signing secret, so the configured one and its version
// stay. The URL and token come back null to a read-scoped token and after the
// server's key rotation, though the token still works, so a null keeps the
// prior ones.
func alertSourceToModel(prior alertSourceModel, s *client.AlertSource) alertSourceModel {
	return alertSourceModel{
		ID:                     types.StringValue(s.ID),
		Name:                   keepReq(prior.Name, s.Name, false),
		Kind:                   types.StringValue(s.Kind),
		Enabled:                types.BoolValue(s.Enabled),
		Grouping:               types.StringValue(s.Grouping),
		EscalationPolicyID:     fromOptString(s.EscalationPolicyID),
		Alerts:                 alertsToModel(s.Alerts),
		NotifyRecovery:         types.BoolValue(s.NotifyRecovery),
		RenotifyIntervalSecs:   types.Int64Value(int64(s.RenotifyIntervalSecs)),
		SigningSecret:          prior.SigningSecret,
		SigningSecretWo:        types.StringNull(),
		SigningSecretWoVersion: prior.SigningSecretWoVersion,
		IngestURL:              keepWhenAbsent(prior.IngestURL, s.IngestURL),
		IngestToken:            keepWhenAbsent(prior.IngestToken, s.IngestToken),
	}
}

// forgetUnsignedSecret is the refresh's view of a secret removed in the app:
// the secret and its version read back null, so the next plan sends it again.
// Only a refresh does this; after an apply the plan's values must stand.
func (m *alertSourceModel) forgetUnsignedSecret(s *client.AlertSource) {
	if !s.Signed {
		m.SigningSecret = types.StringNull()
		m.SigningSecretWoVersion = types.Int64Null()
	}
}

func keepWhenAbsent(prior types.String, got *string) types.String {
	if got == nil && !prior.IsUnknown() {
		return prior
	}
	return fromOptString(got)
}

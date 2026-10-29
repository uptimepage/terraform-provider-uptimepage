package provider

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-go/tftypes"

	"github.com/uptimepage/terraform-provider-uptimepage/internal/client"
)

const importedUUID = "0192f3a4-5b6c-7d8e-9f01-23456789abcd"

// readImported runs Read on the state an import leaves: the id and nothing
// else, nested blocks included.
func readImported(t *testing.T, r resource.Resource) *resource.ReadResponse {
	t.Helper()
	ctx := context.Background()
	sch, objType := resourceSchema(t, r)
	state := tfsdk.State{Schema: sch, Raw: rawWith(objType, "id", tftypes.NewValue(tftypes.String, importedUUID))}
	resp := &resource.ReadResponse{State: state}
	r.Read(ctx, resource.ReadRequest{State: state}, resp)
	return resp
}

type fakeTargetAPI struct {
	targetAPI
	got *client.Target
}

func (f fakeTargetAPI) GetTarget(context.Context, string) (*client.Target, error) {
	return f.got, nil
}

func (f fakeTargetAPI) GetTargetRegions(context.Context, string) ([]string, error) {
	return []string{"eu-helsinki"}, nil
}

func TestTargetImport_ReadsFromTheIDAlone(t *testing.T) {
	bearer := redactedSentinel
	api := fakeTargetAPI{got: &client.Target{
		ID:   importedUUID,
		Name: "api",
		Check: client.CheckSpec{Type: client.CheckTypeHTTP, HTTP: &client.HTTPCheck{
			URL:            "https://example.com/healthz",
			Method:         "GET",
			Timeout:        5000,
			ExpectedStatus: client.ExpectedStatus{Kind: client.StatusKindExact, Exact: 200},
			Headers:        map[string]string{},
			VerifyTLS:      true,
			BearerToken:    &bearer,
		}},
		Interval:  60,
		Enabled:   true,
		Tags:      []string{"prod"},
		Alerts:    []client.AlertBinding{},
		GroupName: strptr("DNS"),
		GroupID:   strptr(groupUUID),
		FiringPolicy: client.FiringPolicy{
			AlertConfirmations: 2, NotifyRecovery: true, RenotifyIntervalSecs: 3600,
			RegionPolicy: &client.RegionPolicy{Mode: client.RegionPolicyMajority},
		},
	}}
	resp := readImported(t, &targetResource{api: api})
	if resp.Diagnostics.HasError() {
		t.Fatalf("read after import: %v", resp.Diagnostics)
	}
	var got targetModel
	resp.Diagnostics.Append(resp.State.Get(context.Background(), &got)...)
	if resp.Diagnostics.HasError() {
		t.Fatalf("state: %v", resp.Diagnostics)
	}
	if got.Name.ValueString() != "api" || got.Check.HTTP == nil || got.Check.HTTP.URL.ValueString() != "https://example.com/healthz" {
		t.Errorf("imported state = %+v", got)
	}
	if got.GroupName.ValueString() != "DNS" || !got.GroupID.IsNull() {
		t.Errorf("group = %v / %v, want it read by name", got.GroupName, got.GroupID)
	}
	if !got.Check.HTTP.BearerToken.IsNull() {
		t.Errorf("a redacted secret must land empty on import, got %v", got.Check.HTTP.BearerToken)
	}
}

type fakeChannelAPI struct {
	channelAPI
	got *client.NotificationChannel
}

func (f fakeChannelAPI) GetChannel(context.Context, string) (*client.NotificationChannel, error) {
	return f.got, nil
}

func TestChannelImport_ReadsFromTheIDAlone(t *testing.T) {
	api := fakeChannelAPI{got: &client.NotificationChannel{
		ID:      importedUUID,
		Name:    "ops slack",
		Kind:    client.ChannelTypeSlack,
		Enabled: true,
		Config: client.ChannelConfig{
			Type:  client.ChannelTypeSlack,
			Slack: &client.SlackConfig{WebhookURL: redactedSentinel, Mention: "@here"},
		},
	}}
	resp := readImported(t, &channelResource{api: api})
	if resp.Diagnostics.HasError() {
		t.Fatalf("read after import: %v", resp.Diagnostics)
	}
	var got channelModel
	resp.Diagnostics.Append(resp.State.Get(context.Background(), &got)...)
	if resp.Diagnostics.HasError() {
		t.Fatalf("state: %v", resp.Diagnostics)
	}
	if got.Name.ValueString() != "ops slack" || got.Config.Slack == nil || got.Config.Slack.Mention.ValueString() != "@here" {
		t.Fatalf("imported state = %+v", got)
	}
	if !got.Config.Slack.WebhookURL.IsNull() {
		t.Errorf("a redacted webhook must land empty on import, got %v", got.Config.Slack.WebhookURL)
	}
}

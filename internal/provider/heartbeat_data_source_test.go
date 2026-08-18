package provider

import (
	"context"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-go/tftypes"

	"github.com/uptimepage/terraform-provider-uptimepage/internal/client"
)

type fakeHeartbeatAPI struct {
	info *client.HeartbeatInfo
	err  error
	got  string
}

func (f *fakeHeartbeatAPI) GetHeartbeat(_ context.Context, id string) (*client.HeartbeatInfo, error) {
	f.got = id
	return f.info, f.err
}

func readHeartbeat(t *testing.T, api heartbeatAPI, id string) (heartbeatDataModel, *datasource.ReadResponse) {
	t.Helper()
	ctx := context.Background()
	d := &heartbeatDataSource{api: api}

	var sresp datasource.SchemaResponse
	d.Schema(ctx, datasource.SchemaRequest{}, &sresp)
	objType := sresp.Schema.Type().TerraformType(ctx).(tftypes.Object)

	raw := tftypes.NewValue(objType, map[string]tftypes.Value{
		"target_id": tftypes.NewValue(tftypes.String, id),
		"ping_url":  tftypes.NewValue(tftypes.String, nil),
	})
	resp := &datasource.ReadResponse{
		State: tfsdk.State{Raw: tftypes.NewValue(objType, nil), Schema: sresp.Schema},
	}
	d.Read(ctx, datasource.ReadRequest{
		Config: tfsdk.Config{Raw: raw, Schema: sresp.Schema},
	}, resp)

	var out heartbeatDataModel
	if !resp.Diagnostics.HasError() {
		resp.Diagnostics.Append(resp.State.Get(ctx, &out)...)
	}
	return out, resp
}

func TestHeartbeatDataSourceExposesThePingURL(t *testing.T) {
	url := "https://app.example.com/ping/abc123"
	api := &fakeHeartbeatAPI{info: &client.HeartbeatInfo{PingURL: &url}}
	id := "11111111-1111-1111-1111-111111111111"
	got, resp := readHeartbeat(t, api, id)
	if resp.Diagnostics.HasError() {
		t.Fatalf("read: %v", resp.Diagnostics)
	}
	if api.got != id {
		t.Errorf("asked the API for %q, want %q", api.got, id)
	}
	if got.PingURL.ValueString() != url {
		t.Errorf("ping_url = %q, want %q", got.PingURL.ValueString(), url)
	}
}

// A token that cannot be decrypted comes back null rather than as an empty
// string, so a config can tell "no URL" from "the URL is blank".
func TestHeartbeatDataSourceNullWhenTheTokenIsUnreadable(t *testing.T) {
	api := &fakeHeartbeatAPI{info: &client.HeartbeatInfo{PingURL: nil}}
	got, resp := readHeartbeat(t, api, "22222222-2222-2222-2222-222222222222")
	if resp.Diagnostics.HasError() {
		t.Fatalf("read: %v", resp.Diagnostics)
	}
	if !got.PingURL.IsNull() {
		t.Errorf("ping_url = %v, want null", got.PingURL)
	}
}

// Pointing it at a non-heartbeat target 404s. A generic "read failed" would
// leave someone guessing, so the diagnostic has to name the cause.
func TestHeartbeatDataSourceExplainsAWrongKind(t *testing.T) {
	api := &fakeHeartbeatAPI{err: &client.APIError{Status: 404, Code: "HEARTBEAT_NOT_CONFIGURED"}}
	_, resp := readHeartbeat(t, api, "33333333-3333-3333-3333-333333333333")
	if !resp.Diagnostics.HasError() {
		t.Fatal("a 404 should be an error")
	}
	if detail := resp.Diagnostics.Errors()[0].Detail(); !strings.Contains(detail, "different check kind") {
		t.Errorf("diagnostic does not explain the cause: %q", detail)
	}
}

// The ping URL is a capability: whoever holds it can report the job healthy or
// failed, so it must not land in plan output.
func TestHeartbeatPingURLIsSensitive(t *testing.T) {
	var resp datasource.SchemaResponse
	(&heartbeatDataSource{}).Schema(context.Background(), datasource.SchemaRequest{}, &resp)
	if !resp.Schema.Attributes["ping_url"].IsSensitive() {
		t.Error("ping_url is not marked sensitive")
	}
}

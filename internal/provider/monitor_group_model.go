package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/uptimepage/terraform-provider-uptimepage/internal/client"
)

// monitorGroupModel is the tfsdk view of an uptimepage_monitor_group.
type monitorGroupModel struct {
	ID                   types.String `tfsdk:"id"`
	Name                 types.String `tfsdk:"name"`
	AlertAsOne           types.Bool   `tfsdk:"alert_as_one"`
	Alerts               []alertModel `tfsdk:"alerts"`
	RenotifyIntervalSecs types.Int64  `tfsdk:"renotify_interval_secs"`
	AlertAgain           types.Bool   `tfsdk:"alert_again"`
	Tags                 types.Set    `tfsdk:"tags"`
}

func (m monitorGroupModel) toInput(ctx context.Context) (client.MonitorGroupInput, diag.Diagnostics) {
	var diags diag.Diagnostics
	out := client.MonitorGroupInput{
		Name:                 m.Name.ValueString(),
		AlertAsOne:           m.AlertAsOne.ValueBool(),
		Alerts:               alertsToWire(m.Alerts),
		RenotifyIntervalSecs: uint32(m.RenotifyIntervalSecs.ValueInt64()),
		AlertAgain:           m.AlertAgain.ValueBool(),
		Tags:                 setStrings(ctx, m.Tags, &diags),
	}
	return out, diags
}

// monitorGroupToModel maps a read group into the model. The API trims the
// name, so the user's spelling is kept when it names the same group.
func monitorGroupToModel(ctx context.Context, prior monitorGroupModel, g *client.MonitorGroup) (monitorGroupModel, diag.Diagnostics) {
	var diags diag.Diagnostics
	out := monitorGroupModel{
		ID:                   types.StringValue(g.ID),
		Name:                 keepReq(prior.Name, g.Name, false),
		AlertAsOne:           types.BoolValue(g.AlertAsOne),
		Alerts:               alertsToModel(g.Alerts),
		RenotifyIntervalSecs: types.Int64Value(int64(g.RenotifyIntervalSecs)),
		AlertAgain:           types.BoolValue(g.AlertAgain),
		Tags:                 stringsToSet(ctx, g.Tags, &diags),
	}
	return out, diags
}

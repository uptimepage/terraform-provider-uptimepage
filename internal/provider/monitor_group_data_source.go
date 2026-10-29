package provider

import (
	"context"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	dschema "github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/uptimepage/terraform-provider-uptimepage/internal/client"
)

type monitorGroupListAPI interface {
	ListGroups(ctx context.Context) ([]client.MonitorGroup, error)
}

var (
	_ datasource.DataSource              = (*monitorGroupDataSource)(nil)
	_ datasource.DataSourceWithConfigure = (*monitorGroupDataSource)(nil)
)

type monitorGroupDataSource struct {
	api monitorGroupListAPI
}

func newMonitorGroupDataSource() datasource.DataSource {
	return &monitorGroupDataSource{}
}

func (d *monitorGroupDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_monitor_group"
}

func (d *monitorGroupDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if c, ok := clientFromProviderData(req.ProviderData, &resp.Diagnostics); ok && c != nil {
		d.api = c
	}
}

func (d *monitorGroupDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = dschema.Schema{
		Description: "Look up a monitor group by name, such as one made in the app or by a monitor's `group_name`.",
		Attributes: map[string]dschema.Attribute{
			"name": dschema.StringAttribute{
				Required:    true,
				Description: "Group name, matched exactly after trimming.",
			},
			"id": dschema.StringAttribute{
				Computed:    true,
				Description: "Group id (UUID), for a target's `group_id`.",
			},
			"alert_as_one": dschema.BoolAttribute{
				Computed:    true,
				Description: "Whether the group pages as one.",
			},
			"renotify_interval_secs": dschema.Int64Attribute{Computed: true},
			"alert_again":            dschema.BoolAttribute{Computed: true},
			"tags": dschema.SetAttribute{
				Computed:    true,
				ElementType: types.StringType,
			},
			"member_count": dschema.Int64Attribute{
				Computed:    true,
				Description: "Monitors in the group.",
			},
		},
	}
}

type monitorGroupDataModel struct {
	Name                 types.String `tfsdk:"name"`
	ID                   types.String `tfsdk:"id"`
	AlertAsOne           types.Bool   `tfsdk:"alert_as_one"`
	RenotifyIntervalSecs types.Int64  `tfsdk:"renotify_interval_secs"`
	AlertAgain           types.Bool   `tfsdk:"alert_again"`
	Tags                 types.Set    `tfsdk:"tags"`
	MemberCount          types.Int64  `tfsdk:"member_count"`
}

func (d *monitorGroupDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var cfg monitorGroupDataModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}
	groups, err := d.api.ListGroups(ctx)
	if err != nil {
		resp.Diagnostics.AddError("List monitor groups failed", err.Error())
		return
	}
	want := strings.TrimSpace(cfg.Name.ValueString())
	for _, g := range groups {
		if g.Name != want {
			continue
		}
		out := monitorGroupDataModel{
			Name:                 cfg.Name,
			ID:                   types.StringValue(g.ID),
			AlertAsOne:           types.BoolValue(g.AlertAsOne),
			RenotifyIntervalSecs: types.Int64Value(int64(g.RenotifyIntervalSecs)),
			AlertAgain:           types.BoolValue(g.AlertAgain),
			Tags:                 stringsToSet(ctx, g.Tags, &resp.Diagnostics),
			MemberCount:          types.Int64Value(int64(g.MemberCount)),
		}
		resp.Diagnostics.Append(resp.State.Set(ctx, &out)...)
		return
	}
	resp.Diagnostics.AddAttributeError(path.Root("name"), "Monitor group not found",
		fmt.Sprintf("No monitor group in this organization is named %q.", want))
}

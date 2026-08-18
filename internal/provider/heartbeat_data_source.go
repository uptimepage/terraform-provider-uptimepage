package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	dschema "github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/uptimepage/terraform-provider-uptimepage/internal/client"
)

// The ping URL lives on its own endpoint rather than in the target payload, and
// the rest of that response is run telemetry that moves on its own. A data
// source keeps both facts out of uptimepage_target: nobody pays for the extra
// request unless they need the URL, and a value that changes between plans is
// expected here rather than a perpetual diff.
type heartbeatAPI interface {
	GetHeartbeat(ctx context.Context, id string) (*client.HeartbeatInfo, error)
}

var (
	_ datasource.DataSource              = (*heartbeatDataSource)(nil)
	_ datasource.DataSourceWithConfigure = (*heartbeatDataSource)(nil)
)

type heartbeatDataSource struct {
	api heartbeatAPI
}

func newHeartbeatDataSource() datasource.DataSource {
	return &heartbeatDataSource{}
}

func (d *heartbeatDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_heartbeat"
}

func (d *heartbeatDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if c, ok := clientFromProviderData(req.ProviderData, &resp.Diagnostics); ok && c != nil {
		d.api = c
	}
}

func (d *heartbeatDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = dschema.Schema{
		Description: "The ping URL of a heartbeat monitor, for the job that reports in. " +
			"The target must be a heartbeat: any other kind is a not-found error.",
		Attributes: map[string]dschema.Attribute{
			"target_id": dschema.StringAttribute{
				Required:    true,
				Description: "Id of the heartbeat target (UUID).",
			},
			"ping_url": dschema.StringAttribute{
				Computed:  true,
				Sensitive: true,
				Description: "URL the job POSTs to. Anyone holding it can report the job " +
					"healthy or failed, so treat it as a credential. Null when the stored " +
					"token cannot be decrypted, which means the monitor needs recreating.",
			},
		},
	}
}

type heartbeatDataModel struct {
	TargetID types.String `tfsdk:"target_id"`
	PingURL  types.String `tfsdk:"ping_url"`
}

func (d *heartbeatDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var cfg heartbeatDataModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}

	got, err := d.api.GetHeartbeat(ctx, cfg.TargetID.ValueString())
	if err != nil {
		if client.IsNotFound(err) {
			resp.Diagnostics.AddError("No heartbeat for that target",
				"The API has no heartbeat for "+cfg.TargetID.ValueString()+
					". Either the id is wrong or the target is a different check kind.")
			return
		}
		resp.Diagnostics.AddError("Read heartbeat failed", err.Error())
		return
	}

	cfg.PingURL = types.StringNull()
	if got.PingURL != nil {
		cfg.PingURL = types.StringValue(*got.PingURL)
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &cfg)...)
}

package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	dschema "github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var (
	_ datasource.DataSource              = (*targetDataSource)(nil)
	_ datasource.DataSourceWithConfigure = (*targetDataSource)(nil)
)

type targetDataSource struct {
	api targetAPI
}

func newTargetDataSource() datasource.DataSource {
	return &targetDataSource{}
}

func (d *targetDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_target"
}

func (d *targetDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if c, ok := clientFromProviderData(req.ProviderData, &resp.Diagnostics); ok && c != nil {
		d.api = c
	}
}

func (d *targetDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = dschema.Schema{
		Description: "Look up a single target by id.",
		Attributes: map[string]dschema.Attribute{
			"id": dschema.StringAttribute{
				Required:    true,
				Description: "Target id (UUID).",
			},
			"name":          dschema.StringAttribute{Computed: true},
			"interval":      dschema.Int64Attribute{Computed: true},
			"enabled":       dschema.BoolAttribute{Computed: true},
			"group_name":    dschema.StringAttribute{Computed: true},
			"owner_user_id": dschema.StringAttribute{Computed: true},
			"tags": dschema.SetAttribute{
				Computed:    true,
				ElementType: types.StringType,
			},
			"regions": dschema.SetAttribute{
				Computed:    true,
				ElementType: types.StringType,
				Description: "Regions this target probes from.",
			},
		},
	}
}

// targetDataModel is the trimmed read-only view exposed by the data source.
type targetDataModel struct {
	ID          types.String `tfsdk:"id"`
	Name        types.String `tfsdk:"name"`
	Interval    types.Int64  `tfsdk:"interval"`
	Enabled     types.Bool   `tfsdk:"enabled"`
	GroupName   types.String `tfsdk:"group_name"`
	OwnerUserID types.String `tfsdk:"owner_user_id"`
	Tags        types.Set    `tfsdk:"tags"`
	Regions     types.Set    `tfsdk:"regions"`
}

func (d *targetDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var cfg targetDataModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}

	got, err := d.api.GetTarget(ctx, cfg.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Read target failed", err.Error())
		return
	}

	tags, diags := types.SetValueFrom(ctx, types.StringType, got.Tags)
	resp.Diagnostics.Append(diags...)

	regionIDs, err := d.api.GetTargetRegions(ctx, cfg.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Read target regions failed", err.Error())
		return
	}
	regions, rdiags := types.SetValueFrom(ctx, types.StringType, regionIDs)
	resp.Diagnostics.Append(rdiags...)

	out := targetDataModel{
		ID:          types.StringValue(got.ID),
		Name:        types.StringValue(got.Name),
		Interval:    types.Int64Value(int64(got.Interval)),
		Enabled:     types.BoolValue(got.Enabled),
		GroupName:   fromOptString(got.GroupName),
		OwnerUserID: fromOptString(got.OwnerUserID),
		Tags:        tags,
		Regions:     regions,
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &out)...)
}

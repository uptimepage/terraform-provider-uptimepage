package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	dschema "github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/uptimepage/terraform-provider-uptimepage/internal/client"
)

// The catalog is operator-defined and grows, so a config that hard-codes ids
// dates itself and cannot move to a self-hosted install.
type regionsAPI interface {
	ListRegions(ctx context.Context) ([]client.Region, error)
}

var (
	_ datasource.DataSource              = (*regionsDataSource)(nil)
	_ datasource.DataSourceWithConfigure = (*regionsDataSource)(nil)
)

type regionsDataSource struct {
	api regionsAPI
}

func newRegionsDataSource() datasource.DataSource {
	return &regionsDataSource{}
}

func (d *regionsDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_regions"
}

func (d *regionsDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if c, ok := clientFromProviderData(req.ProviderData, &resp.Diagnostics); ok && c != nil {
		d.api = c
	}
}

func (d *regionsDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = dschema.Schema{
		Description: "The probe regions this instance can check from. Disabled regions are " +
			"absent, so `ids` is exactly the set `uptimepage_target.regions` accepts. " +
			"Assigning every region is still capped by the plan, and a monitor that " +
			"names more regions than the plan pays for is rejected at apply.",
		Attributes: map[string]dschema.Attribute{
			"ids": dschema.SetAttribute{
				Computed:    true,
				ElementType: types.StringType,
				Description: "Region ids, for feeding straight into a target's regions set.",
			},
			"regions": dschema.ListNestedAttribute{
				Computed:    true,
				Description: "The full catalog entry for each region, ordered as the API returns it.",
				NestedObject: dschema.NestedAttributeObject{
					Attributes: map[string]dschema.Attribute{
						"id": dschema.StringAttribute{
							Computed:    true,
							Description: "Stable slug, e.g. \"eu-helsinki\".",
						},
						"name": dschema.StringAttribute{
							Computed:    true,
							Description: "Operator-set display name.",
						},
						"city": dschema.StringAttribute{
							Computed:    true,
							Description: "City the probes run from, when the operator recorded one.",
						},
						"country_code": dschema.StringAttribute{
							Computed:    true,
							Description: "ISO 3166-1 alpha-2 country code, when recorded.",
						},
						"continent": dschema.StringAttribute{
							Computed: true,
							Description: "One of africa, asia, europe, north_america, south_america, " +
								"oceania, antarctica, when recorded.",
						},
						"latitude": dschema.Float64Attribute{
							Computed:    true,
							Description: "Latitude, when recorded. Set together with longitude.",
						},
						"longitude": dschema.Float64Attribute{
							Computed:    true,
							Description: "Longitude, when recorded. Set together with latitude.",
						},
					},
				},
			},
		},
	}
}

type regionModel struct {
	ID          types.String  `tfsdk:"id"`
	Name        types.String  `tfsdk:"name"`
	City        types.String  `tfsdk:"city"`
	CountryCode types.String  `tfsdk:"country_code"`
	Continent   types.String  `tfsdk:"continent"`
	Latitude    types.Float64 `tfsdk:"latitude"`
	Longitude   types.Float64 `tfsdk:"longitude"`
}

type regionsDataModel struct {
	IDs     types.Set     `tfsdk:"ids"`
	Regions []regionModel `tfsdk:"regions"`
}

func regionsToModel(regions []client.Region) []regionModel {
	out := make([]regionModel, 0, len(regions))
	for _, r := range regions {
		out = append(out, regionModel{
			ID:          types.StringValue(r.ID),
			Name:        types.StringValue(r.Name),
			City:        optStringOrNull(r.City),
			CountryCode: optStringOrNull(r.CountryCode),
			Continent:   optStringOrNull(r.Continent),
			Latitude:    fromOptFloat64(r.Latitude),
			Longitude:   fromOptFloat64(r.Longitude),
		})
	}
	return out
}

// 0 is a real latitude, so the zero value cannot stand in for "not recorded".
func fromOptFloat64(v *float64) types.Float64 {
	if v == nil {
		return types.Float64Null()
	}
	return types.Float64Value(*v)
}

func (d *regionsDataSource) Read(ctx context.Context, _ datasource.ReadRequest, resp *datasource.ReadResponse) {
	regions, err := d.api.ListRegions(ctx)
	if err != nil {
		resp.Diagnostics.AddError("List regions failed", err.Error())
		return
	}

	ids := make([]string, 0, len(regions))
	for _, r := range regions {
		ids = append(ids, r.ID)
	}
	idSet, diags := types.SetValueFrom(ctx, types.StringType, ids)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &regionsDataModel{
		IDs:     idSet,
		Regions: regionsToModel(regions),
	})...)
}

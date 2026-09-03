package provider

import (
	"context"
	"errors"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-go/tftypes"

	"github.com/uptimepage/terraform-provider-uptimepage/internal/client"
)

type fakeRegionsAPI struct {
	regions []client.Region
	err     error
}

func (f *fakeRegionsAPI) ListRegions(_ context.Context) ([]client.Region, error) {
	return f.regions, f.err
}

func readRegions(t *testing.T, api regionsAPI) (regionsDataModel, *datasource.ReadResponse) {
	t.Helper()
	ctx := context.Background()
	d := &regionsDataSource{api: api}

	var sresp datasource.SchemaResponse
	d.Schema(ctx, datasource.SchemaRequest{}, &sresp)
	objType := sresp.Schema.Type().TerraformType(ctx).(tftypes.Object)

	resp := &datasource.ReadResponse{
		State: tfsdk.State{Raw: tftypes.NewValue(objType, nil), Schema: sresp.Schema},
	}
	d.Read(ctx, datasource.ReadRequest{
		Config: tfsdk.Config{Raw: tftypes.NewValue(objType, nil), Schema: sresp.Schema},
	}, resp)

	var out regionsDataModel
	if !resp.Diagnostics.HasError() {
		resp.Diagnostics.Append(resp.State.Get(ctx, &out)...)
	}
	return out, resp
}

func TestRegionsDataSourceCarriesIDsAndTheCatalog(t *testing.T) {
	lat, lon := 60.17, 24.94
	api := &fakeRegionsAPI{regions: []client.Region{
		{
			ID: "eu-helsinki", Name: "EU North", City: "Helsinki",
			CountryCode: "FI", Continent: "europe", Latitude: &lat, Longitude: &lon,
		},
		{ID: "lab", Name: "Lab"},
	}}

	got, resp := readRegions(t, api)
	if resp.Diagnostics.HasError() {
		t.Fatalf("read: %v", resp.Diagnostics)
	}
	if n := len(got.IDs.Elements()); n != 2 {
		t.Fatalf("ids = %d entries, want 2", n)
	}
	if got.Regions[0].City.ValueString() != "Helsinki" {
		t.Errorf("city = %q, want Helsinki", got.Regions[0].City.ValueString())
	}
	if got.Regions[0].Latitude.ValueFloat64() != lat {
		t.Errorf("latitude = %v, want %v", got.Regions[0].Latitude, lat)
	}
	// Null, not "" and not 0: "not recorded" differs from a value on the equator.
	bare := got.Regions[1]
	for name, null := range map[string]bool{
		"city":         bare.City.IsNull(),
		"country_code": bare.CountryCode.IsNull(),
		"continent":    bare.Continent.IsNull(),
		"latitude":     bare.Latitude.IsNull(),
		"longitude":    bare.Longitude.IsNull(),
	} {
		if !null {
			t.Errorf("%s is set on a region that carries none", name)
		}
	}
}

func TestRegionsDataSourceReportsAFailedRead(t *testing.T) {
	_, resp := readRegions(t, &fakeRegionsAPI{err: errors.New("boom")})
	if !resp.Diagnostics.HasError() {
		t.Fatal("a failed catalog read must surface, not leave empty state")
	}
}

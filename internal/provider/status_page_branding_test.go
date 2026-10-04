package provider

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

// Branding goes out whole, so an omitted attribute planned unknown would be
// sent as null and reset a theme or accent picked in the console. Planning the
// page's current value keeps it.
func TestStatusPageBranding_OmittedConfigKeepsThePageValue(t *testing.T) {
	sch, objType := resourceSchema(t, &statusPageResource{})
	for attr, value := range map[string]string{"style": "dark", "brand_color": "#0a7cff"} {
		resp := planString(t, sch, path.Root(attr), planmodifier.StringRequest{
			State:       tfsdk.State{Raw: rawWith(objType, attr, tftypes.NewValue(tftypes.String, value))},
			Plan:        tfsdk.Plan{Raw: rawNull(objType)},
			ConfigValue: types.StringNull(),
			StateValue:  types.StringValue(value),
			PlanValue:   types.StringUnknown(),
		})
		if resp.Diagnostics.HasError() {
			t.Fatal(resp.Diagnostics)
		}
		if resp.PlanValue.ValueString() != value {
			t.Errorf("%s plan = %v, want the page's %s", attr, resp.PlanValue, value)
		}
	}
}

// A null prior stays unknown, so the server's answer fills it rather than a
// planned null.
func TestStatusPageBranding_NullPriorStaysUnknown(t *testing.T) {
	sch, objType := resourceSchema(t, &statusPageResource{})
	for _, attr := range []string{"style", "brand_color"} {
		resp := planString(t, sch, path.Root(attr), planmodifier.StringRequest{
			State:       tfsdk.State{Raw: rawNull(objType)},
			Plan:        tfsdk.Plan{Raw: rawNull(objType)},
			ConfigValue: types.StringNull(),
			StateValue:  types.StringNull(),
			PlanValue:   types.StringUnknown(),
		})
		if resp.Diagnostics.HasError() {
			t.Fatal(resp.Diagnostics)
		}
		if !resp.PlanValue.IsUnknown() {
			t.Errorf("%s plan = %v, want unknown", attr, resp.PlanValue)
		}
	}
}

func TestStatusPageHideFromSearch_OmittedConfigKeepsThePageValue(t *testing.T) {
	sch, objType := resourceSchema(t, &statusPageResource{})
	resp := planBool(t, sch, path.Root("hide_from_search"), planmodifier.BoolRequest{
		State:       tfsdk.State{Raw: rawWith(objType, "hide_from_search", tftypes.NewValue(tftypes.Bool, true))},
		Plan:        tfsdk.Plan{Raw: rawNull(objType)},
		ConfigValue: types.BoolNull(),
		StateValue:  types.BoolValue(true),
		PlanValue:   types.BoolUnknown(),
	})
	if resp.Diagnostics.HasError() {
		t.Fatal(resp.Diagnostics)
	}
	if !resp.PlanValue.ValueBool() {
		t.Errorf("plan = %v, want the page's true", resp.PlanValue)
	}
}

func TestStatusPageHideFromSearch_NullPriorStaysUnknown(t *testing.T) {
	sch, objType := resourceSchema(t, &statusPageResource{})
	resp := planBool(t, sch, path.Root("hide_from_search"), planmodifier.BoolRequest{
		State:       tfsdk.State{Raw: rawNull(objType)},
		Plan:        tfsdk.Plan{Raw: rawNull(objType)},
		ConfigValue: types.BoolNull(),
		StateValue:  types.BoolNull(),
		PlanValue:   types.BoolUnknown(),
	})
	if resp.Diagnostics.HasError() {
		t.Fatal(resp.Diagnostics)
	}
	if !resp.PlanValue.IsUnknown() {
		t.Errorf("plan = %v, want unknown", resp.PlanValue)
	}
}

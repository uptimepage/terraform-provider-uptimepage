package provider

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

// statusPageRaw builds a whole-resource value with the given slug; every other
// attribute is null, which the modifier never reads.
func statusPageRaw(t *testing.T, objType tftypes.Object, slug string) tftypes.Value {
	t.Helper()
	attrs := map[string]tftypes.Value{}
	for name, typ := range objType.AttributeTypes {
		attrs[name] = tftypes.NewValue(typ, nil)
	}
	attrs["slug"] = tftypes.NewValue(tftypes.String, slug)
	return tftypes.NewValue(objType, attrs)
}

// planStatusURL runs the modifier over an unknown status_url. An empty
// stateSlug stands for a create, where there is no prior state at all.
func planStatusURL(t *testing.T, stateSlug, planSlug string, priorURL types.String) planmodifier.StringResponse {
	t.Helper()
	ctx := context.Background()

	var sresp resource.SchemaResponse
	(&statusPageResource{}).Schema(ctx, resource.SchemaRequest{}, &sresp)
	objType := sresp.Schema.Type().TerraformType(ctx).(tftypes.Object)

	state := tfsdk.State{Raw: tftypes.NewValue(objType, nil), Schema: sresp.Schema}
	if stateSlug != "" {
		state.Raw = statusPageRaw(t, objType, stateSlug)
	}

	req := planmodifier.StringRequest{
		Path:       path.Root("status_url"),
		State:      state,
		Plan:       tfsdk.Plan{Raw: statusPageRaw(t, objType, planSlug), Schema: sresp.Schema},
		StateValue: priorURL,
		PlanValue:  types.StringUnknown(),
	}
	resp := planmodifier.StringResponse{PlanValue: req.PlanValue}
	keepWhileSlugUnchanged().PlanModifyString(ctx, req, &resp)
	return resp
}

func TestSlugDerivedURLHoldsValueWhileSlugUnchanged(t *testing.T) {
	url := "https://acme.uptimepage.dev"
	resp := planStatusURL(t, "acme", "acme", types.StringValue(url))
	if resp.Diagnostics.HasError() {
		t.Fatalf("modify: %v", resp.Diagnostics)
	}
	if resp.PlanValue.IsUnknown() || resp.PlanValue.ValueString() != url {
		t.Errorf("branding-only update should keep %q, got %v", url, resp.PlanValue)
	}
}

func TestSlugDerivedURLStaysUnknownOnSlugChange(t *testing.T) {
	resp := planStatusURL(t, "acme", "acme-2", types.StringValue("https://acme.uptimepage.dev"))
	if !resp.PlanValue.IsUnknown() {
		t.Errorf("a new slug means a new URL only the API can spell, got %v", resp.PlanValue)
	}
}

func TestSlugDerivedURLStaysUnknownOnCreate(t *testing.T) {
	resp := planStatusURL(t, "", "acme", types.StringNull())
	if !resp.PlanValue.IsUnknown() {
		t.Errorf("create has no prior URL, got %v", resp.PlanValue)
	}
}

func TestSlugDerivedURLStaysUnknownWhenNoURLWasMounted(t *testing.T) {
	resp := planStatusURL(t, "acme", "acme", types.StringNull())
	if !resp.PlanValue.IsUnknown() {
		t.Errorf("a page with no public surface has no URL to hold, got %v", resp.PlanValue)
	}
}

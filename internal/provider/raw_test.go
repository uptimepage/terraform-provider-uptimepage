package provider

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

// resourceSchema returns a resource's schema and its whole-resource object
// type, the two things every raw value and plan modifier under test needs.
func resourceSchema(t *testing.T, r resource.Resource) (schema.Schema, tftypes.Object) {
	t.Helper()
	ctx := context.Background()
	var resp resource.SchemaResponse
	r.Schema(ctx, resource.SchemaRequest{}, &resp)
	return resp.Schema, resp.Schema.Type().TerraformType(ctx).(tftypes.Object)
}

func nullAttrs(objType tftypes.Object) map[string]tftypes.Value {
	attrs := map[string]tftypes.Value{}
	for n, typ := range objType.AttributeTypes {
		attrs[n] = tftypes.NewValue(typ, nil)
	}
	return attrs
}

// rawNull builds a whole-resource value with every attribute null. It is still
// a non-null resource, which is all a modifier reads to tell an update from a
// create or a destroy.
func rawNull(objType tftypes.Object) tftypes.Value {
	return tftypes.NewValue(objType, nullAttrs(objType))
}

// rawWith builds a whole-resource value with one attribute set and every other
// one null, which is all a plan modifier under test ever reads.
func rawWith(objType tftypes.Object, name string, value tftypes.Value) tftypes.Value {
	attrs := nullAttrs(objType)
	attrs[name] = value
	return tftypes.NewValue(objType, attrs)
}

// planString runs the string attribute at p through its plan modifiers the way
// fwserver does: one response per modifier, each starting from the plan value
// the previous one left, with RequiresReplace and diagnostics accumulated.
// req carries the whole-resource raws under State and Plan (a null state raw
// is a create, a null plan raw a destroy) and the attribute's own values.
func planString(t *testing.T, sch schema.Schema, p path.Path, req planmodifier.StringRequest) planmodifier.StringResponse {
	t.Helper()
	ctx := context.Background()
	attr, diags := sch.AttributeAtPath(ctx, p)
	if diags.HasError() {
		t.Fatalf("attribute at %s: %v", p, diags)
	}
	req.Path = p
	req.State.Schema = sch
	req.Plan.Schema = sch
	resp := planmodifier.StringResponse{PlanValue: req.PlanValue}
	for _, m := range attr.(schema.StringAttribute).PlanModifiers {
		step := planmodifier.StringResponse{PlanValue: req.PlanValue}
		m.PlanModifyString(ctx, req, &step)
		req.PlanValue = step.PlanValue
		resp.PlanValue = step.PlanValue
		resp.RequiresReplace = resp.RequiresReplace || step.RequiresReplace
		resp.Diagnostics.Append(step.Diagnostics...)
	}
	return resp
}

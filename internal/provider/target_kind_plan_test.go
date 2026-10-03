package provider

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

// planCheckType runs check.type's plan modifiers. A null stateKind is a
// create, a null planKind a destroy; anything else is an update between the
// two kinds.
func planCheckType(t *testing.T, stateKind, planKind types.String) planmodifier.StringResponse {
	t.Helper()
	sch, objType := resourceSchema(t, &targetResource{})
	raw := func(kind types.String) tftypes.Value {
		if kind.IsNull() {
			return tftypes.NewValue(objType, nil)
		}
		return rawNull(objType)
	}
	return planString(t, sch, path.Root("check").AtName("type"), planmodifier.StringRequest{
		State:      tfsdk.State{Raw: raw(stateKind)},
		Plan:       tfsdk.Plan{Raw: raw(planKind)},
		StateValue: stateKind,
		PlanValue:  planKind,
	})
}

// The API refuses a check of another kind on an existing monitor, so a type
// edit has to plan as a replacement rather than fail at apply.
func TestCheckTypeChangeReplacesTheMonitor(t *testing.T) {
	tcp, http := types.StringValue("tcp"), types.StringValue("http")
	if resp := planCheckType(t, tcp, http); !resp.RequiresReplace {
		t.Error("tcp -> http should replace the monitor")
	}
	if resp := planCheckType(t, tcp, tcp); resp.RequiresReplace {
		t.Error("an unchanged type must not replace the monitor")
	}
	if resp := planCheckType(t, types.StringNull(), tcp); resp.RequiresReplace {
		t.Error("a create has nothing to replace")
	}
	if resp := planCheckType(t, tcp, types.StringNull()); resp.RequiresReplace {
		t.Error("a destroy is not a replacement")
	}
}

// planConfirmations runs alert_confirmations' plan modifiers on a config whose
// check is the given value, starting from the plan value the schema default
// left. A null state is a create.
func planConfirmations(t *testing.T, check func(tftypes.Object) tftypes.Value, state tftypes.Value, config, planned types.Int64) types.Int64 {
	t.Helper()
	sch, objType := resourceSchema(t, &targetResource{})
	cfg := rawWith(objType, "check", check(objType.AttributeTypes["check"].(tftypes.Object)))
	if state.Type() == nil {
		state = tftypes.NewValue(objType, nil)
	}
	resp := planInt64(t, sch, path.Root("alert_confirmations"), planmodifier.Int64Request{
		Config:      tfsdk.Config{Raw: cfg},
		Plan:        tfsdk.Plan{Raw: cfg},
		State:       tfsdk.State{Raw: state},
		ConfigValue: config,
		PlanValue:   planned,
	})
	if resp.Diagnostics.HasError() {
		t.Fatalf("modifier: %v", resp.Diagnostics)
	}
	return resp.PlanValue
}

func checkOfType(kind any) func(tftypes.Object) tftypes.Value {
	return func(typ tftypes.Object) tftypes.Value {
		attrs := nullAttrs(typ)
		attrs["type"] = tftypes.NewValue(tftypes.String, kind)
		return tftypes.NewValue(typ, attrs)
	}
}

// The API keeps 1 for a manual monitor whatever it is sent, so the default of
// 2 would read back as 1 and fail the apply.
func TestManualMonitorPlansOneConfirmation(t *testing.T) {
	_, objType := resourceSchema(t, &targetResource{})
	created := tftypes.Value{}
	two := types.Int64Value(2)
	unknownCheck := func(typ tftypes.Object) tftypes.Value { return tftypes.NewValue(typ, tftypes.UnknownValue) }
	for _, c := range []struct {
		check  func(tftypes.Object) tftypes.Value
		state  tftypes.Value
		config types.Int64
		want   types.Int64
		why    string
	}{
		{checkOfType("manual"), created, types.Int64Null(), types.Int64Value(1), "manual left out"},
		{checkOfType("manual"), rawNull(objType), types.Int64Null(), types.Int64Value(1), "manual left out on an update or import"},
		{checkOfType("http"), created, types.Int64Null(), two, "another kind keeps the default"},
		{checkOfType("manual"), created, types.Int64Value(3), types.Int64Value(3), "an explicit count is the validator's to refuse"},
		{checkOfType(tftypes.UnknownValue), created, types.Int64Null(), types.Int64Unknown(), "type unknown at plan"},
		{unknownCheck, created, types.Int64Null(), types.Int64Unknown(), "whole check unknown at plan"},
	} {
		t.Run(c.why, func(t *testing.T) {
			planned := two
			if !c.config.IsNull() {
				planned = c.config
			}
			if got := planConfirmations(t, c.check, c.state, c.config, planned); !got.Equal(c.want) {
				t.Errorf("planned %v, want %v", got, c.want)
			}
		})
	}
}

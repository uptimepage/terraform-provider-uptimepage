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

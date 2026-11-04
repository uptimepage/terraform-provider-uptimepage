package provider

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"

	"github.com/uptimepage/terraform-provider-uptimepage/internal/client"
)

func TestTimezoneValidator(t *testing.T) {
	check := func(name string) bool {
		var resp validator.StringResponse
		timezoneValidator{}.ValidateString(context.Background(), validator.StringRequest{
			Path:        path.Root("timezone"),
			ConfigValue: types.StringValue(name),
		}, &resp)
		return !resp.Diagnostics.HasError()
	}
	for _, name := range []string{"UTC", "Europe/Berlin", "America/Sao_Paulo", "US/Eastern"} {
		if !check(name) {
			t.Errorf("%q refused", name)
		}
	}
	for _, name := range []string{"", "Local", "UTC+2", "Mars/Olympus", "Europe/Berlinn"} {
		if check(name) {
			t.Errorf("%q accepted", name)
		}
	}
}

func TestStatusPageToUpdate_SendsTimezoneBesideBranding(t *testing.T) {
	body, err := json.Marshal(statusPageModel{
		Slug:     types.StringValue("acme"),
		Name:     types.StringValue("Acme"),
		Timezone: types.StringValue("Europe/Berlin"),
	}.toUpdate())
	if err != nil {
		t.Fatal(err)
	}
	var wire map[string]any
	if err := json.Unmarshal(body, &wire); err != nil {
		t.Fatal(err)
	}
	if wire["public_timezone"] != "Europe/Berlin" {
		t.Errorf("public_timezone = %v, want top-level Europe/Berlin", wire["public_timezone"])
	}
}

// An unknown or unset zone leaves the field off the wire, which the API reads
// as "keep the page's zone".
func TestStatusPageToUpdate_OmitsAnUndecidedTimezone(t *testing.T) {
	for _, zone := range []types.String{types.StringNull(), types.StringUnknown()} {
		body, err := json.Marshal(statusPageModel{
			Slug:     types.StringValue("acme"),
			Name:     types.StringValue("Acme"),
			Timezone: zone,
		}.toUpdate())
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(body), "public_timezone") {
			t.Errorf("%v zone must not be sent: %s", zone, body)
		}
	}
}

func TestStatusPageToModel_ReadsTimezone(t *testing.T) {
	berlin := "Europe/Berlin"
	got := statusPageToModel(statusPageModel{}, &client.StatusPage{
		ID: "id", Slug: "acme", Name: "Acme", PublicStyle: "default", PublicTimezone: &berlin,
	})
	if got.Timezone.ValueString() != berlin {
		t.Errorf("timezone = %v, want %s", got.Timezone, berlin)
	}
	// A server that predates the field reads as unset, not as "".
	older := statusPageToModel(statusPageModel{}, &client.StatusPage{
		ID: "id", Slug: "acme", Name: "Acme", PublicStyle: "default",
	})
	if !older.Timezone.IsNull() {
		t.Errorf("timezone = %v, want null", older.Timezone)
	}
}

// A config that never names the zone plans the page's current one, so a zone
// picked in the console survives the next apply.
func TestStatusPageTimezone_OmittedConfigKeepsThePageZone(t *testing.T) {
	sch, objType := resourceSchema(t, &statusPageResource{})
	raw := rawWith(objType, "timezone", tftypes.NewValue(tftypes.String, "Europe/Berlin"))
	resp := planString(t, sch, path.Root("timezone"), planmodifier.StringRequest{
		State:       tfsdk.State{Raw: raw},
		Plan:        tfsdk.Plan{Raw: rawNull(objType)},
		ConfigValue: types.StringNull(),
		StateValue:  types.StringValue("Europe/Berlin"),
		PlanValue:   types.StringUnknown(),
	})
	if resp.Diagnostics.HasError() {
		t.Fatal(resp.Diagnostics)
	}
	if resp.PlanValue.ValueString() != "Europe/Berlin" {
		t.Errorf("plan = %v, want the page's Europe/Berlin", resp.PlanValue)
	}
}

// State written by a provider without the attribute holds null. Planning that
// null would contradict the "UTC" the server answers.
func TestStatusPageTimezone_NullPriorStaysUnknown(t *testing.T) {
	sch, objType := resourceSchema(t, &statusPageResource{})
	resp := planString(t, sch, path.Root("timezone"), planmodifier.StringRequest{
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
		t.Errorf("plan = %v, want unknown", resp.PlanValue)
	}
}

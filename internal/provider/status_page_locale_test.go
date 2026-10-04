package provider

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"

	"github.com/uptimepage/terraform-provider-uptimepage/internal/client"
)

func TestStatusPageToUpdate_SendsLocaleBesideBranding(t *testing.T) {
	up := statusPageModel{
		Slug:   types.StringValue("acme"),
		Name:   types.StringValue("Acme"),
		Locale: types.StringValue("de"),
	}.toUpdate()
	body, err := json.Marshal(up)
	if err != nil {
		t.Fatal(err)
	}
	var wire map[string]any
	if err := json.Unmarshal(body, &wire); err != nil {
		t.Fatal(err)
	}
	if wire["public_locale"] != "de" {
		t.Errorf("public_locale = %v, want top-level de", wire["public_locale"])
	}
}

// An unknown or unset language leaves the field off the wire, which the API
// reads as "keep the page's language".
func TestStatusPageToUpdate_OmitsAnUndecidedLocale(t *testing.T) {
	for _, locale := range []types.String{types.StringNull(), types.StringUnknown()} {
		body, err := json.Marshal(statusPageModel{
			Slug:   types.StringValue("acme"),
			Name:   types.StringValue("Acme"),
			Locale: locale,
		}.toUpdate())
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(body), "public_locale") {
			t.Errorf("%v locale must not be sent: %s", locale, body)
		}
	}
}

func TestStatusPageToModel_ReadsLocale(t *testing.T) {
	de := "de"
	got := statusPageToModel(statusPageModel{}, &client.StatusPage{
		ID: "id", Slug: "acme", Name: "Acme", PublicStyle: "default", PublicLocale: &de,
	})
	if got.Locale.ValueString() != "de" {
		t.Errorf("locale = %v, want de", got.Locale)
	}
	// A server that predates the field reads as unset, not as "".
	older := statusPageToModel(statusPageModel{}, &client.StatusPage{
		ID: "id", Slug: "acme", Name: "Acme", PublicStyle: "default",
	})
	if !older.Locale.IsNull() {
		t.Errorf("locale = %v, want null", older.Locale)
	}
}

// A config that never names the language plans the page's current one, so a
// language picked in the console survives the next apply.
func TestStatusPageLocale_OmittedConfigKeepsThePageLanguage(t *testing.T) {
	sch, objType := resourceSchema(t, &statusPageResource{})
	raw := rawWith(objType, "locale", tftypes.NewValue(tftypes.String, "de"))
	resp := planString(t, sch, path.Root("locale"), planmodifier.StringRequest{
		State:       tfsdk.State{Raw: raw},
		Plan:        tfsdk.Plan{Raw: rawNull(objType)},
		ConfigValue: types.StringNull(),
		StateValue:  types.StringValue("de"),
		PlanValue:   types.StringUnknown(),
	})
	if resp.Diagnostics.HasError() {
		t.Fatal(resp.Diagnostics)
	}
	if resp.PlanValue.ValueString() != "de" {
		t.Errorf("plan = %v, want the page's de", resp.PlanValue)
	}
}

// State written by a provider without the attribute holds null. Planning that
// null would contradict the "en" the server answers.
func TestStatusPageLocale_NullPriorStaysUnknown(t *testing.T) {
	sch, objType := resourceSchema(t, &statusPageResource{})
	resp := planString(t, sch, path.Root("locale"), planmodifier.StringRequest{
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

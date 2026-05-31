package provider

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/uptimepage/terraform-provider-uptimepage/internal/client"
)

func TestStatusPageToUpdate_BrandingPointers(t *testing.T) {
	m := statusPageModel{
		Slug:        types.StringValue("acme"),
		Name:        types.StringValue("Acme"),
		Enabled:     types.BoolValue(true),
		DisplayName: types.StringNull(), // unset -> nil -> clears server-side
		About:       types.StringValue("All systems"),
		Style:       types.StringValue("dark"),
		// BrandColor + ShowPoweredBy left unknown -> nil (server default).
		BrandColor:    types.StringNull(),
		ShowPoweredBy: types.BoolNull(),
	}
	up := m.toUpdate()
	if up.Branding.PublicDisplayName != nil {
		t.Errorf("display name should be nil (clear), got %v", *up.Branding.PublicDisplayName)
	}
	if up.Branding.PublicAbout == nil || *up.Branding.PublicAbout != "All systems" {
		t.Errorf("about = %v", up.Branding.PublicAbout)
	}
	if up.Branding.PublicStyle == nil || *up.Branding.PublicStyle != "dark" {
		t.Errorf("style = %v", up.Branding.PublicStyle)
	}
	if up.Branding.PublicShowPoweredBy != nil {
		t.Errorf("show_powered_by should be nil, got %v", *up.Branding.PublicShowPoweredBy)
	}
}

func TestStatusPageToModel_KeepsSlugCaseAndComputed(t *testing.T) {
	prior := statusPageModel{Slug: types.StringValue("Acme")} // user wrote mixed case
	got := statusPageToModel(prior, &client.StatusPage{
		ID:               "p1",
		Slug:             "acme", // API canonicalized to lowercase
		Name:             "Acme",
		Enabled:          true,
		PublicBrandColor: strptr("#0a0"),
		PublicStyle:      "dark",
		ShowPoweredBy:    true,
		StatusURL:        strptr("https://acme.example.com"),
	})
	if got.Slug.ValueString() != "Acme" {
		t.Errorf("slug = %q, want preserved mixed-case Acme", got.Slug.ValueString())
	}
	if got.BrandColor.ValueString() != "#0a0" || got.Style.ValueString() != "dark" {
		t.Errorf("computed branding not mapped: %+v", got)
	}
	if got.StatusURL.ValueString() != "https://acme.example.com" {
		t.Errorf("status_url = %q", got.StatusURL.ValueString())
	}
	if got.LogoURL.IsNull() != true {
		t.Errorf("logo_url should be null when API omits it")
	}
}

func TestStatusPageToModel_KeepsCanonicalizedInput(t *testing.T) {
	// The user's config values; the API trims, lowercases the brand color, and
	// drops blanks to null. State must keep the user's spelling when equivalent,
	// or Terraform reports "inconsistent result after apply" / diffs forever.
	prior := statusPageModel{
		Slug:        types.StringValue(" Acme "),  // server: "acme"
		Name:        types.StringValue("Acme "),   // server: "Acme"
		DisplayName: types.StringValue("Ops "),    // server: "Ops"
		About:       types.StringValue("   "),     // server: null (blank)
		BrandColor:  types.StringValue("#AABBCC"), // server: "#aabbcc"
	}
	got := statusPageToModel(prior, &client.StatusPage{
		ID: "p1", Slug: "acme", Name: "Acme",
		PublicDisplayName: strptr("Ops"),
		PublicAbout:       nil,
		PublicBrandColor:  strptr("#aabbcc"),
		PublicStyle:       "default",
	})
	if got.Slug.ValueString() != " Acme " {
		t.Errorf("slug = %q, want kept user form", got.Slug.ValueString())
	}
	if got.Name.ValueString() != "Acme " {
		t.Errorf("name = %q, want kept user form", got.Name.ValueString())
	}
	if got.DisplayName.ValueString() != "Ops " {
		t.Errorf("display_name = %q, want kept user form", got.DisplayName.ValueString())
	}
	if got.About.ValueString() != "   " {
		t.Errorf("about = %q, want kept blank user form (server null)", got.About.ValueString())
	}
	if got.BrandColor.ValueString() != "#AABBCC" {
		t.Errorf("brand_color = %q, want kept user case", got.BrandColor.ValueString())
	}
}

func TestStatusPageToModel_SurfacesGenuineDrift(t *testing.T) {
	prior := statusPageModel{
		Name:       types.StringValue("Acme"),
		BrandColor: types.StringValue("#aaaaaa"),
	}
	got := statusPageToModel(prior, &client.StatusPage{
		ID: "p1", Slug: "acme", Name: "Renamed Out Of Band",
		PublicBrandColor: strptr("#bbbbbb"),
		PublicStyle:      "default",
	})
	if got.Name.ValueString() != "Renamed Out Of Band" {
		t.Errorf("name drift not surfaced: %q", got.Name.ValueString())
	}
	if got.BrandColor.ValueString() != "#bbbbbb" {
		t.Errorf("brand_color drift not surfaced: %q", got.BrandColor.ValueString())
	}
}

func TestStatusPageToModel_ShowPoweredByTriState(t *testing.T) {
	// Pinned false must read back as false (not collapse to the resolved value),
	// and an absent override must read back as null (inherit) — no diff either way.
	pinned := statusPageToModel(statusPageModel{}, &client.StatusPage{
		ID: "p1", PublicStyle: "default",
		PublicShowPoweredBy: boolptr(false), ShowPoweredBy: false,
	})
	if pinned.ShowPoweredBy.IsNull() || pinned.ShowPoweredBy.ValueBool() != false {
		t.Errorf("pinned false = %v, want known false", pinned.ShowPoweredBy)
	}
	inherit := statusPageToModel(statusPageModel{}, &client.StatusPage{
		ID: "p1", PublicStyle: "default",
		PublicShowPoweredBy: nil, ShowPoweredBy: true, // resolved true, but unset
	})
	if !inherit.ShowPoweredBy.IsNull() {
		t.Errorf("unset override = %v, want null", inherit.ShowPoweredBy)
	}
}

func boolptr(b bool) *bool { return &b }

func TestComponentToModel_KeepsCanonicalizedCuration(t *testing.T) {
	prior := statusPageComponentModel{
		PageID:      types.StringValue("p1"),
		PublicName:  types.StringValue("API "), // server: "API"
		PublicGroup: types.StringValue("  "),   // server: null
	}
	got := componentToModel(prior, &client.StatusPageComponent{
		TargetID: "t1", MonitorName: "api", PublicName: strptr("API"), PublicGroup: nil,
	})
	if got.PublicName.ValueString() != "API " {
		t.Errorf("public_name = %q, want kept user form", got.PublicName.ValueString())
	}
	if got.PublicGroup.ValueString() != "  " {
		t.Errorf("public_group = %q, want kept blank user form", got.PublicGroup.ValueString())
	}
}

func TestComponentModel_RoundTrip(t *testing.T) {
	m := statusPageComponentModel{
		PageID:      types.StringValue("p1"),
		TargetID:    types.StringValue("t1"),
		PublicName:  types.StringValue("API"),
		PublicGroup: types.StringNull(), // -> nil -> clears
		SortOrder:   types.Int64Value(2),
	}
	in := m.toNew()
	if in.TargetID != "t1" || in.PublicName == nil || *in.PublicName != "API" || in.SortOrder != 2 {
		t.Fatalf("toNew = %+v", in)
	}
	if in.PublicGroup != nil {
		t.Errorf("public_group should be nil")
	}
	upd := m.toUpdate()
	if upd.PublicGroup != nil || upd.SortOrder != 2 {
		t.Errorf("toUpdate = %+v", upd)
	}

	got := componentToModel(m, &client.StatusPageComponent{
		TargetID: "t1", MonitorName: "API monitor", PublicName: strptr("API"), SortOrder: 2,
	})
	if got.ID.ValueString() != "p1:t1" {
		t.Errorf("id = %q, want p1:t1", got.ID.ValueString())
	}
	if got.MonitorName.ValueString() != "API monitor" {
		t.Errorf("monitor_name = %q", got.MonitorName.ValueString())
	}
}

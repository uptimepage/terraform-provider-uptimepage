package provider

import (
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/uptimepage/terraform-provider-uptimepage/internal/client"
)

// statusPageModel is the tfsdk view of an uptimepage_status_page. Branding
// fields are flattened onto the resource; logo_url / status_url are read-only.
type statusPageModel struct {
	ID            types.String `tfsdk:"id"`
	Slug          types.String `tfsdk:"slug"`
	Name          types.String `tfsdk:"name"`
	Enabled       types.Bool   `tfsdk:"enabled"`
	DisplayName   types.String `tfsdk:"display_name"`
	About         types.String `tfsdk:"about"`
	BrandColor    types.String `tfsdk:"brand_color"`
	Style         types.String `tfsdk:"style"`
	ShowPoweredBy types.Bool   `tfsdk:"show_powered_by"`
	LogoURL       types.String `tfsdk:"logo_url"`
	StatusURL     types.String `tfsdk:"status_url"`
}

func (m statusPageModel) toNew() client.NewStatusPage {
	return client.NewStatusPage{
		Slug:    m.Slug.ValueString(),
		Name:    m.Name.ValueString(),
		Enabled: m.Enabled.ValueBool(),
	}
}

func (m statusPageModel) toUpdate() client.StatusPageUpdate {
	return client.StatusPageUpdate{
		Name:    m.Name.ValueString(),
		Slug:    m.Slug.ValueString(),
		Enabled: m.Enabled.ValueBool(),
		Branding: client.StatusBranding{
			PublicDisplayName:   optString(m.DisplayName),
			PublicAbout:         optString(m.About),
			PublicBrandColor:    optString(m.BrandColor),
			PublicStyle:         optString(m.Style),
			PublicShowPoweredBy: optBool(m.ShowPoweredBy),
		},
	}
}

// statusPageToModel maps a read page into the model. prior preserves the user's
// slug spelling when it differs from the API's canonical form by case only, so
// a `MyPage` -> `mypage` lowercasing doesn't diff forever.
func statusPageToModel(prior statusPageModel, p *client.StatusPage) statusPageModel {
	return statusPageModel{
		ID:            types.StringValue(p.ID),
		Slug:          keepReq(prior.Slug, p.Slug, true),
		Name:          keepReq(prior.Name, p.Name, false),
		Enabled:       types.BoolValue(p.Enabled),
		DisplayName:   keepOpt(prior.DisplayName, p.PublicDisplayName, false),
		About:         keepOpt(prior.About, p.PublicAbout, false),
		BrandColor:    keepOpt(prior.BrandColor, p.PublicBrandColor, true),
		Style:         types.StringValue(p.PublicStyle),
		ShowPoweredBy: fromOptBool(p.PublicShowPoweredBy),
		LogoURL:       fromOptString(p.LogoURL),
		StatusURL:     fromOptString(p.StatusURL),
	}
}

// optBool mirrors optString for a tri-state bool: null/unknown -> nil (the API
// applies its default), set -> the value.
func optBool(b types.Bool) *bool {
	if b.IsNull() || b.IsUnknown() {
		return nil
	}
	v := b.ValueBool()
	return &v
}

// fromOptBool mirrors fromOptString: nil -> null (override absent, inherit), set
// -> the pinned value.
func fromOptBool(b *bool) types.Bool {
	if b == nil {
		return types.BoolNull()
	}
	return types.BoolValue(*b)
}

// The API canonicalizes user strings before storing them: it trims whitespace,
// drops blanks to null, and lowercases the slug and brand color. Echoing the
// canonical form back into state where it differs from the user's config value
// trips Terraform's post-apply consistency check (and then diffs forever). The
// keep* helpers preserve the user's spelling whenever it is canonically
// equivalent to what the server returned, and surface the server value only on
// genuine drift — mirroring the existing keepURL/keepSecret pattern.

// canonEq reports whether the user's string canonicalizes to the server's value.
func canonEq(user, server string, fold bool) bool {
	u := strings.TrimSpace(user)
	if fold {
		return strings.EqualFold(u, server)
	}
	return u == server
}

// keepReq handles a Required (always-present) string: keep prior when it
// canonicalizes to got, else reflect the server value.
func keepReq(prior types.String, got string, fold bool) types.String {
	if !prior.IsNull() && !prior.IsUnknown() && canonEq(prior.ValueString(), got, fold) {
		return prior
	}
	return types.StringValue(got)
}

// keepOpt handles an Optional nullable string. got is the server value (nil =
// absent/blank). Keep prior when it canonicalizes to got — including the case
// where the user wrote blank/whitespace and the server stored null.
func keepOpt(prior types.String, got *string, fold bool) types.String {
	if prior.IsNull() || prior.IsUnknown() {
		return fromOptString(got)
	}
	server := ""
	if got != nil {
		server = *got
	}
	if canonEq(prior.ValueString(), server, fold) {
		return prior
	}
	return fromOptString(got)
}

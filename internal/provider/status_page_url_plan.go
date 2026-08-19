package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// slugDerivedURL holds a computed URL at its prior value while the slug stays
// put. The public page URL is the slug plus deployment shape and nothing else,
// so leaving it unknown on a branding-only update is plan noise; on a slug
// change it stays unknown because only the API can spell the new one. Not for
// the logo URL, whose content-hash query moves on an upload the page update
// never sees.
type slugDerivedURL struct{}

func keepWhileSlugUnchanged() planmodifier.String { return slugDerivedURL{} }

func (slugDerivedURL) Description(_ context.Context) string {
	return "Holds the value from state while slug is unchanged."
}

func (m slugDerivedURL) MarkdownDescription(ctx context.Context) string {
	return m.Description(ctx)
}

func (slugDerivedURL) PlanModifyString(ctx context.Context, req planmodifier.StringRequest, resp *planmodifier.StringResponse) {
	// Create and destroy have no prior value to carry over.
	if req.State.Raw.IsNull() || req.Plan.Raw.IsNull() {
		return
	}
	// A page with no public surface has no prior URL to hold.
	if req.StateValue.IsNull() || !req.PlanValue.IsUnknown() {
		return
	}

	var stateSlug, planSlug types.String
	resp.Diagnostics.Append(req.State.GetAttribute(ctx, path.Root("slug"), &stateSlug)...)
	resp.Diagnostics.Append(req.Plan.GetAttribute(ctx, path.Root("slug"), &planSlug)...)
	if resp.Diagnostics.HasError() || planSlug.IsUnknown() || !stateSlug.Equal(planSlug) {
		return
	}
	resp.PlanValue = req.StateValue
}

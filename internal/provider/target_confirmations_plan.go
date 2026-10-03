package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/uptimepage/terraform-provider-uptimepage/internal/client"
)

// manualConfirmationsPlan plans the one count the API keeps for a manual
// monitor whose config leaves alert_confirmations out. The schema default of 2
// would be stored as 1 and fail the apply as an inconsistent result. While the
// type is unknown the count is too, since either answer could be wrong.
type manualConfirmationsPlan struct{}

func oneConfirmationWhenManual() planmodifier.Int64 { return manualConfirmationsPlan{} }

func (manualConfirmationsPlan) Description(_ context.Context) string {
	return "Plans 1 for a manual monitor that leaves the count out."
}

func (m manualConfirmationsPlan) MarkdownDescription(ctx context.Context) string {
	return m.Description(ctx)
}

func (manualConfirmationsPlan) PlanModifyInt64(ctx context.Context, req planmodifier.Int64Request, resp *planmodifier.Int64Response) {
	if !req.ConfigValue.IsNull() {
		return
	}
	// Read through the object: a type under an unknown check reads as null.
	var check types.Object
	resp.Diagnostics.Append(req.Config.GetAttribute(ctx, path.Root("check"), &check)...)
	if check.IsNull() {
		return
	}
	kind, _ := check.Attributes()["type"].(types.String)
	switch {
	case check.IsUnknown() || kind.IsUnknown():
		resp.PlanValue = types.Int64Unknown()
	case kind.ValueString() == client.CheckTypeManual:
		resp.PlanValue = types.Int64Value(manualConfirmations)
	}
}

package provider

import (
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/uptimepage/terraform-provider-uptimepage/internal/client"
)

// statusPageComponentModel is the tfsdk view of one monitor curated onto a page.
// The composite id is "{status_page_id}:{target_id}"; monitor_name is read-only.
type statusPageComponentModel struct {
	ID                types.String `tfsdk:"id"`
	PageID            types.String `tfsdk:"status_page_id"`
	TargetID          types.String `tfsdk:"target_id"`
	PublicName        types.String `tfsdk:"public_name"`
	PublicDescription types.String `tfsdk:"public_description"`
	PublicGroup       types.String `tfsdk:"public_group"`
	SortOrder         types.Int64  `tfsdk:"sort_order"`
	MonitorName       types.String `tfsdk:"monitor_name"`
}

func (m statusPageComponentModel) toNew() client.NewStatusPageComponent {
	return client.NewStatusPageComponent{
		TargetID:          m.TargetID.ValueString(),
		PublicName:        optString(m.PublicName),
		PublicDescription: optString(m.PublicDescription),
		PublicGroup:       optString(m.PublicGroup),
		SortOrder:         int(m.SortOrder.ValueInt64()),
	}
}

func (m statusPageComponentModel) toUpdate() client.StatusPageComponentUpdate {
	return client.StatusPageComponentUpdate{
		PublicName:        optString(m.PublicName),
		PublicDescription: optString(m.PublicDescription),
		PublicGroup:       optString(m.PublicGroup),
		SortOrder:         int(m.SortOrder.ValueInt64()),
	}
}

// componentToModel maps a read component into the model, preserving the prior
// page/target identity (the API echoes target_id but not the page).
func componentToModel(prior statusPageComponentModel, c *client.StatusPageComponent) statusPageComponentModel {
	// The API trims the curation fields and drops blanks to null; keep the
	// user's form when canonically equivalent so a trailing space / blank
	// doesn't trip the post-apply consistency check or diff forever.
	return statusPageComponentModel{
		ID:                types.StringValue(componentID(prior.PageID.ValueString(), c.TargetID)),
		PageID:            prior.PageID,
		TargetID:          types.StringValue(c.TargetID),
		PublicName:        keepOpt(prior.PublicName, c.PublicName, false),
		PublicDescription: keepOpt(prior.PublicDescription, c.PublicDescription, false),
		PublicGroup:       keepOpt(prior.PublicGroup, c.PublicGroup, false),
		SortOrder:         types.Int64Value(int64(c.SortOrder)),
		MonitorName:       types.StringValue(c.MonitorName),
	}
}

func componentID(pageID, targetID string) string {
	return pageID + ":" + targetID
}

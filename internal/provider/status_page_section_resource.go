package provider

import (
	"context"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/uptimepage/terraform-provider-uptimepage/internal/client"
)

type sectionAPI interface {
	CreateSection(ctx context.Context, pageID string, in client.NewStatusPageSection) (*client.StatusPageSection, error)
	ListSections(ctx context.Context, pageID string) ([]client.StatusPageSection, error)
	UpdateSection(ctx context.Context, pageID, sectionID string, in client.StatusPageSectionUpdate) error
	DeleteSection(ctx context.Context, pageID, sectionID string) error
}

var (
	_ resource.Resource                = (*statusPageSectionResource)(nil)
	_ resource.ResourceWithConfigure   = (*statusPageSectionResource)(nil)
	_ resource.ResourceWithImportState = (*statusPageSectionResource)(nil)
)

type statusPageSectionResource struct {
	api sectionAPI
}

func newStatusPageSectionResource() resource.Resource {
	return &statusPageSectionResource{}
}

// statusPageSectionModel is the tfsdk view of a group shown on a page.
type statusPageSectionModel struct {
	ID      types.String `tfsdk:"id"`
	PageID  types.String `tfsdk:"status_page_id"`
	GroupID types.String `tfsdk:"group_id"`
	Heading types.String `tfsdk:"heading"`
}

func (r *statusPageSectionResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_status_page_section"
}

func (r *statusPageSectionResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if c, ok := clientFromProviderData(req.ProviderData, &resp.Diagnostics); ok && c != nil {
		r.api = c
	}
}

func (r *statusPageSectionResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	replace := []planmodifier.String{stringplanmodifier.RequiresReplace()}
	resp.Schema = schema.Schema{
		Description: "Shows a monitor group on a status page as a live section: a heading with the members' combined state, and each member as its own component. " +
			"The section mirrors the group, so a monitor that joins the group appears on the page and one that leaves it is taken off. " +
			"A member already on the page moves into the section with its settings; manage a member's public name and order with uptimepage_status_page_component, which adopts the member's row. " +
			"Members past the plan's public component limit stay off the page, and the apply warns. Destroying or replacing the section takes its members off the page; a component that adopted one comes back on the next apply. Owner-only.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:      true,
				Description:   "Section id (UUID).",
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"status_page_id": schema.StringAttribute{
				Required:      true,
				Description:   "Id of the status page (lowercase UUID).",
				PlanModifiers: replace,
				Validators:    []validator.String{uuidValidator()},
			},
			"group_id": schema.StringAttribute{
				Required:      true,
				Description:   "Id of the monitor group to show (lowercase UUID). A group has at most one section on a page.",
				PlanModifiers: replace,
				Validators:    []validator.String{uuidValidator()},
			},
			"heading": schema.StringAttribute{
				Optional:    true,
				Description: "Public heading, at most 50 characters. Leave it out to show the group's name; set one when that name is operator-facing.",
			},
		},
	}
}

func (r *statusPageSectionResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan statusPageSectionModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	got, err := r.api.CreateSection(ctx, plan.PageID.ValueString(), client.NewStatusPageSection{
		GroupID: plan.GroupID.ValueString(),
		Heading: optString(plan.Heading),
	})
	if err != nil {
		resp.Diagnostics.AddError("Add status page section failed", err.Error())
		return
	}
	if got.LeftOff > 0 {
		resp.Diagnostics.AddWarning("Some members are not on the page",
			fmt.Sprintf("%d of the group's monitors are left off: the plan's public component limit is reached. "+
				"Free slots or upgrade, then replace this section to add them.", got.LeftOff))
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, sectionToModel(plan, got))...)
}

func (r *statusPageSectionResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state statusPageSectionModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	got, err := r.findSection(ctx, state.PageID.ValueString(), state.ID.ValueString())
	if err != nil {
		if client.IsNotFound(err) { // the page itself is gone
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Read status page section failed", err.Error())
		return
	}
	if got == nil { // removed in the app, or its group was deleted
		resp.State.RemoveResource(ctx)
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, sectionToModel(state, got))...)
}

func (r *statusPageSectionResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan statusPageSectionModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	err := r.api.UpdateSection(ctx, plan.PageID.ValueString(), plan.ID.ValueString(),
		client.StatusPageSectionUpdate{Heading: optString(plan.Heading)})
	if err != nil {
		resp.Diagnostics.AddError("Update status page section failed", err.Error())
		return
	}
	// The heading is the only thing an update changes, and the API stores it
	// as the plan reads once trimmed.
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *statusPageSectionResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state statusPageSectionModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	err := r.api.DeleteSection(ctx, state.PageID.ValueString(), state.ID.ValueString())
	if err != nil && !client.IsNotFound(err) {
		resp.Diagnostics.AddError("Remove status page section failed", err.Error())
	}
}

func (r *statusPageSectionResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	pageID, sectionID, ok := strings.Cut(req.ID, ":")
	if !ok || pageID == "" || sectionID == "" {
		resp.Diagnostics.AddError("Invalid import id",
			fmt.Sprintf("expected `status_page_id:section_id`, got %q", req.ID))
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("status_page_id"), pageID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), sectionID)...)
}

// findSection returns the section on pageID, or nil if the page exists but no
// longer shows it. A missing page surfaces as an IsNotFound error.
func (r *statusPageSectionResource) findSection(ctx context.Context, pageID, sectionID string) (*client.StatusPageSection, error) {
	list, err := r.api.ListSections(ctx, pageID)
	if err != nil {
		return nil, err
	}
	for i := range list {
		if list[i].ID == sectionID {
			return &list[i], nil
		}
	}
	return nil, nil
}

// sectionToModel keeps the page from prior: the section read does not echo it.
// The API trims the heading and drops a blank one, so the user's spelling is
// kept when it reads the same.
func sectionToModel(prior statusPageSectionModel, s *client.StatusPageSection) statusPageSectionModel {
	return statusPageSectionModel{
		ID:      types.StringValue(s.ID),
		PageID:  prior.PageID,
		GroupID: types.StringValue(s.GroupID),
		Heading: keepOpt(prior.Heading, s.Heading, false),
	}
}

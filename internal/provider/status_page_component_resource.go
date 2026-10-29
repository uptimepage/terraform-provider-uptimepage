package provider

import (
	"context"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/uptimepage/terraform-provider-uptimepage/internal/client"
)

type componentAPI interface {
	AddComponent(ctx context.Context, pageID string, in client.NewStatusPageComponent) error
	ListComponents(ctx context.Context, pageID string) ([]client.StatusPageComponent, error)
	UpdateComponent(ctx context.Context, pageID, targetID string, in client.StatusPageComponentUpdate) error
	RemoveComponent(ctx context.Context, pageID, targetID string) error
}

var (
	_ resource.Resource                = (*statusPageComponentResource)(nil)
	_ resource.ResourceWithConfigure   = (*statusPageComponentResource)(nil)
	_ resource.ResourceWithImportState = (*statusPageComponentResource)(nil)
)

type statusPageComponentResource struct {
	api componentAPI
}

func newStatusPageComponentResource() resource.Resource {
	return &statusPageComponentResource{}
}

func (r *statusPageComponentResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_status_page_component"
}

func (r *statusPageComponentResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if c, ok := clientFromProviderData(req.ProviderData, &resp.Diagnostics); ok && c != nil {
		r.api = c
	}
}

func (r *statusPageComponentResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	replace := []planmodifier.String{stringplanmodifier.RequiresReplace()}
	resp.Schema = schema.Schema{
		Description: "Curates one monitor onto a status page, with optional per-page " +
			"overrides. Changing the page or the target replaces the resource. " +
			"A monitor an uptimepage_status_page_section shows is already on the page: " +
			"this resource then adopts its row and manages only its settings, and " +
			"destroying it clears them and leaves the monitor on the page.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:      true,
				Description:   "Composite id, `status_page_id:target_id`.",
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"status_page_id": schema.StringAttribute{
				Required:      true,
				Description:   "Id of the status page (UUID).",
				PlanModifiers: replace,
			},
			"target_id": schema.StringAttribute{
				Required:      true,
				Description:   "Id of the monitor to curate (UUID).",
				PlanModifiers: replace,
			},
			"public_name": schema.StringAttribute{
				Optional:    true,
				Description: "Per-page display name. Falls back to the monitor name when unset.",
			},
			"public_description": schema.StringAttribute{
				Optional:    true,
				Description: "Per-page description shown under the component.",
			},
			"public_group": schema.StringAttribute{
				Optional:    true,
				Description: "Group heading the component renders under. A section member renders under its section's heading instead.",
			},
			"sort_order": schema.Int64Attribute{
				Optional:      true,
				Computed:      true,
				Description:   "Order within the group (ascending). Left out, a new component takes 0 and a section member keeps its place; removing it later keeps the order it has.",
				PlanModifiers: []planmodifier.Int64{int64planmodifier.UseStateForUnknown()},
			},
			"monitor_name": schema.StringAttribute{
				Computed:    true,
				Description: "The monitor's operator-side name (read-only).",
			},
			"section_id": schema.StringAttribute{
				Computed:    true,
				Description: "The section that shows this monitor through its group, or null (read-only).",
			},
		},
	}
}

func (r *statusPageComponentResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan statusPageComponentModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	pageID := plan.PageID.ValueString()
	if err := r.add(ctx, pageID, plan); err != nil {
		resp.Diagnostics.AddError("Add status page component failed", err.Error())
		return
	}
	// Add returns 204; read the component back to capture monitor_name and any
	// server normalization.
	got, err := r.findComponent(ctx, pageID, plan.TargetID.ValueString())
	if err != nil {
		// The component was added but the read-back failed. Persist a fallback
		// so a re-apply reconciles via Read instead of re-adding into a 409.
		fallback := plan
		fallback.ID = types.StringValue(componentID(pageID, plan.TargetID.ValueString()))
		fallback.MonitorName = types.StringNull()
		fallback.SectionID = types.StringNull()
		if fallback.SortOrder.IsUnknown() {
			fallback.SortOrder = types.Int64Null()
		}
		resp.Diagnostics.Append(resp.State.Set(ctx, fallback)...)
		resp.Diagnostics.AddError("Read component after add failed", err.Error())
		return
	}
	if got == nil {
		resp.Diagnostics.AddError("Component missing after add",
			"the API reported success but the component was not found on the page")
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, componentToModel(plan, got))...)
}

func (r *statusPageComponentResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state statusPageComponentModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	got, err := r.findComponent(ctx, state.PageID.ValueString(), state.TargetID.ValueString())
	if err != nil {
		if client.IsNotFound(err) { // the page itself is gone
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Read status page component failed", err.Error())
		return
	}
	if got == nil { // page exists, component no longer on it
		resp.State.RemoveResource(ctx)
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, componentToModel(state, got))...)
}

func (r *statusPageComponentResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan statusPageComponentModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	pageID, targetID := plan.PageID.ValueString(), plan.TargetID.ValueString()
	if err := r.api.UpdateComponent(ctx, pageID, targetID, plan.toUpdate()); err != nil {
		resp.Diagnostics.AddError("Update status page component failed", err.Error())
		return
	}
	got, err := r.findComponent(ctx, pageID, targetID)
	if err != nil {
		// The PATCH already applied, so the plan is the new truth — persist it
		// (monitor_name re-resolves on the next Read) rather than leaving stale
		// state behind a reported failure.
		fallback := plan
		fallback.ID = types.StringValue(componentID(pageID, targetID))
		fallback.MonitorName = types.StringNull()
		fallback.SectionID = types.StringNull()
		if fallback.SortOrder.IsUnknown() {
			fallback.SortOrder = types.Int64Null()
		}
		resp.Diagnostics.Append(resp.State.Set(ctx, fallback)...)
		resp.Diagnostics.AddError("Read component after update failed", err.Error())
		return
	}
	if got == nil {
		resp.State.RemoveResource(ctx)
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, componentToModel(plan, got))...)
}

func (r *statusPageComponentResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state statusPageComponentModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.remove(ctx, state.PageID.ValueString(), state.TargetID.ValueString()); err != nil {
		resp.Diagnostics.AddError("Remove status page component failed", err.Error())
	}
}

// remove takes the monitor off the page, or clears its settings when a section
// shows it: the section keeps it there until the monitor leaves the group.
func (r *statusPageComponentResource) remove(ctx context.Context, pageID, targetID string) error {
	err := r.api.RemoveComponent(ctx, pageID, targetID)
	if client.HasCode(err, client.CodeSectionMember) {
		// An empty update clears the overrides and leaves the order.
		err = r.api.UpdateComponent(ctx, pageID, targetID, client.StatusPageComponentUpdate{})
	}
	if client.IsNotFound(err) {
		return nil
	}
	return err
}

// add puts the monitor on the page, or adopts the row a section already gave
// it: the section owns that row, so only its settings are this resource's.
func (r *statusPageComponentResource) add(ctx context.Context, pageID string, plan statusPageComponentModel) error {
	err := r.api.AddComponent(ctx, pageID, plan.toNew())
	if !client.HasCode(err, client.CodeComponentAlreadyOnPage) {
		return err
	}
	got, ferr := r.findComponent(ctx, pageID, plan.TargetID.ValueString())
	if ferr != nil {
		return ferr
	}
	if got == nil || got.SectionID == nil {
		return err
	}
	return r.api.UpdateComponent(ctx, pageID, plan.TargetID.ValueString(), plan.toUpdate())
}

func (r *statusPageComponentResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	pageID, targetID, ok := strings.Cut(req.ID, ":")
	if !ok || pageID == "" || targetID == "" {
		resp.Diagnostics.AddError("Invalid import id",
			fmt.Sprintf("expected `status_page_id:target_id`, got %q", req.ID))
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("status_page_id"), pageID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("target_id"), targetID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), componentID(pageID, targetID))...)
}

// findComponent returns the component for targetID on pageID, or nil if the page
// exists but no longer carries it. A missing page surfaces as an IsNotFound error.
func (r *statusPageComponentResource) findComponent(ctx context.Context, pageID, targetID string) (*client.StatusPageComponent, error) {
	list, err := r.api.ListComponents(ctx, pageID)
	if err != nil {
		return nil, err
	}
	for i := range list {
		if list[i].TargetID == targetID {
			return &list[i], nil
		}
	}
	return nil, nil
}

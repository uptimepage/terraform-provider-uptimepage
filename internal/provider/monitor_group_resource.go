package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64default"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/setdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/uptimepage/terraform-provider-uptimepage/internal/client"
)

type monitorGroupAPI interface {
	CreateGroup(ctx context.Context, in client.MonitorGroupInput) (*client.MonitorGroup, error)
	GetGroup(ctx context.Context, id string) (*client.MonitorGroup, error)
	UpdateGroup(ctx context.Context, id string, in client.MonitorGroupInput) (*client.MonitorGroup, error)
	DeleteGroup(ctx context.Context, id string) error
}

var (
	_ resource.Resource                = (*monitorGroupResource)(nil)
	_ resource.ResourceWithConfigure   = (*monitorGroupResource)(nil)
	_ resource.ResourceWithImportState = (*monitorGroupResource)(nil)
)

type monitorGroupResource struct {
	api monitorGroupAPI
}

func newMonitorGroupResource() resource.Resource {
	return &monitorGroupResource{}
}

func (r *monitorGroupResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_monitor_group"
}

func (r *monitorGroupResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if c, ok := clientFromProviderData(req.ProviderData, &resp.Diagnostics); ok && c != nil {
		r.api = c
	}
}

func (r *monitorGroupResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "A monitor group. Monitors join it with `group_id` on uptimepage_target. " +
			"By default a group is a label; with alert_as_one it pages as a single thing: the first member failure opens one group incident, " +
			"later failures join it, and the members' own channels stay quiet. Deleting it keeps its monitors, ungrouped.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:      true,
				Description:   "Group id (UUID).",
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"name": schema.StringAttribute{
				Required:    true,
				Description: "Group name, unique in the organization, at most 50 characters after trimming.",
			},
			"alert_as_one": schema.BoolAttribute{
				Optional: true,
				Computed: true,
				Default:  booldefault.StaticBool(false),
				Description: "Whether the group pages as one. On, a member's failure opens or joins the group incident, which pages this group's alerts and reminder; " +
					"the members' own alerts, reminder and escalation policy stay quiet, and the group's tags stand in for theirs where channel tag rules match. " +
					"Off, each monitor alerts on its own. A switch changes only failures that start after it: an incident open at the time ends the way it started.",
			},
			"alerts": alertsAttribute("Notification channels the group incident pages while the group alerts as one. They also receive its monitoring-stopped notices. While an escalation policy applies to the group (bound in the app or over the API, or the organization default), the policy pages its levels instead."),
			"renotify_interval_secs": schema.Int64Attribute{
				Optional:    true,
				Computed:    true,
				Default:     int64default.StaticInt64(3600),
				Description: "Seconds before the first reminder while the group incident stays unacknowledged; each further reminder waits twice as long, up to a day. 0 turns reminders off; otherwise at least 60.",
				Validators:  renotifyValidators(),
			},
			"alert_again": schema.BoolAttribute{
				Optional:    true,
				Computed:    true,
				Default:     booldefault.StaticBool(true),
				Description: "Whether a member that goes down while the group incident is open sends a message. Messages are coalesced to at most one a minute, naming every member down since the last one. Off, a later failure only adds a line to the incident's timeline. An acknowledgement stays either way.",
			},
			"tags": schema.SetAttribute{
				Optional:    true,
				Computed:    true,
				ElementType: types.StringType,
				Validators:  tagValidators(),
				Default:     setdefault.StaticValue(types.SetValueMust(types.StringType, []attr.Value{})),
				Description: "Tags the members carry while the group alerts as one, in place of their own, for channel tag rules and the tag filter. A member's own `tags` attribute keeps its own tags, so it shows no drift.",
			},
		},
	}
}

func (r *monitorGroupResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan monitorGroupModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	in, d := plan.toInput(ctx)
	resp.Diagnostics.Append(d...)
	if resp.Diagnostics.HasError() {
		return
	}
	created, err := r.api.CreateGroup(ctx, in)
	if err != nil {
		resp.Diagnostics.AddError("Create monitor group failed", err.Error())
		return
	}
	state, d := monitorGroupToModel(ctx, plan, created)
	resp.Diagnostics.Append(d...)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *monitorGroupResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state monitorGroupModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	got, err := r.api.GetGroup(ctx, state.ID.ValueString())
	if err != nil {
		if client.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Read monitor group failed", err.Error())
		return
	}
	next, d := monitorGroupToModel(ctx, state, got)
	resp.Diagnostics.Append(d...)
	resp.Diagnostics.Append(resp.State.Set(ctx, &next)...)
}

func (r *monitorGroupResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan monitorGroupModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	in, d := plan.toInput(ctx)
	resp.Diagnostics.Append(d...)
	if resp.Diagnostics.HasError() {
		return
	}
	updated, err := r.api.UpdateGroup(ctx, plan.ID.ValueString(), in)
	if err != nil {
		resp.Diagnostics.AddError("Update monitor group failed", err.Error())
		return
	}
	next, d := monitorGroupToModel(ctx, plan, updated)
	resp.Diagnostics.Append(d...)
	resp.Diagnostics.Append(resp.State.Set(ctx, &next)...)
}

func (r *monitorGroupResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state monitorGroupModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.api.DeleteGroup(ctx, state.ID.ValueString()); err != nil && !client.IsNotFound(err) {
		resp.Diagnostics.AddError("Delete monitor group failed", err.Error())
	}
}

func (r *monitorGroupResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

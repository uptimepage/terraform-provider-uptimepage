package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64default"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/uptimepage/terraform-provider-uptimepage/internal/client"
)

// The API's bounds, in characters.
const (
	maxAlertSourceNameChars = 100
	minSigningSecretChars   = 16
	maxSigningSecretChars   = 256
)

type alertSourceAPI interface {
	CreateAlertSource(ctx context.Context, in client.NewAlertSource) (*client.AlertSource, error)
	GetAlertSource(ctx context.Context, id string) (*client.AlertSource, error)
	UpdateAlertSource(ctx context.Context, id string, in client.AlertSourceUpdate) (*client.AlertSource, error)
	DeleteAlertSource(ctx context.Context, id string) error
}

var (
	_ resource.Resource                   = (*alertSourceResource)(nil)
	_ resource.ResourceWithConfigure      = (*alertSourceResource)(nil)
	_ resource.ResourceWithImportState    = (*alertSourceResource)(nil)
	_ resource.ResourceWithValidateConfig = (*alertSourceResource)(nil)
)

type alertSourceResource struct {
	api alertSourceAPI
}

func newAlertSourceResource() resource.Resource {
	return &alertSourceResource{}
}

func (r *alertSourceResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_alert_source"
}

func (r *alertSourceResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if c, ok := clientFromProviderData(req.ProviderData, &resp.Diagnostics); ok && c != nil {
		r.api = c
	}
}

func (r *alertSourceResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	secretLength := stringvalidator.UTF8LengthBetween(minSigningSecretChars, maxSigningSecretChars)
	resp.Schema = schema.Schema{
		Description: "An alert source: a URL that Prometheus Alertmanager, Grafana, Datadog or a script posts its alerts to. " +
			"A firing alert opens an incident that pages through the source's escalation policy, the organization default, or its channels, and the tool's recovery closes it. " +
			"Needs a plan with alert sources. Deleting it stops the URL at once and resolves the incidents it holds open.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:      true,
				Description:   "Alert source id (UUID).",
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"name": schema.StringAttribute{
				Required:    true,
				Description: "Source name, unique in the organization, at most 100 characters after trimming. Incidents and pages name it.",
				Validators:  []validator.String{trimmedValidator{}, stringvalidator.UTF8LengthAtMost(maxAlertSourceNameChars)},
			},
			"kind": schema.StringAttribute{
				Required:      true,
				Description:   "The tool that posts: `alertmanager`, `grafana`, `datadog`, or `generic` for a script. Fixed at create, so a change replaces the source: its URL stops, the incidents it holds open are resolved, and a policy bound in the app is not carried over. Replacing needs `alert_sources:delete`.",
				Validators:    []validator.String{stringvalidator.OneOf(client.AlertSourceKindAlertmanager, client.AlertSourceKindGrafana, client.AlertSourceKindDatadog, client.AlertSourceKindGeneric)},
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"enabled": schema.BoolAttribute{
				Optional:    true,
				Computed:    true,
				Default:     booldefault.StaticBool(true),
				Description: "Off pauses the source: a firing opens nothing and the tool still gets a 202, while a recovery still closes. Nothing is queued, so an alert still firing on resume opens on its next repeat.",
			},
			"grouping": schema.StringAttribute{
				Optional: true,
				Computed: true,
				Default:  stringdefault.StaticString(client.AlertGroupingGroup),
				Description: "`group` (the default) opens one incident per alert group the tool sends, so a failure across many hosts pages once; `alert` opens one per alert. " +
					"It can change only while the source holds no open incident (409 ALERT_SOURCE_GROUPING_LOCKED otherwise).",
				Validators: []validator.String{stringvalidator.OneOf(client.AlertGroupingGroup, client.AlertGroupingAlert)},
			},
			"escalation_policy_id": schema.StringAttribute{
				Optional: true,
				Computed: true,
				Description: "Escalation policy the source's incidents page (UUID, from the policy's page in the app). Binding or changing it needs a token of an organization owner with `oncall:write`. " +
					"Leave it out to bind the policy in the app instead, or to page the organization default policy or the source's channels; removing the line keeps the binding it set.",
				Validators:    []validator.String{uuidValidator()},
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"alerts": alertsAttribute("Notification channels the source's incidents page when no escalation policy applies, neither its own nor the organization default. Terraform owns the list, so a channel bound in the app is removed on the next apply. A source must reach someone, so with no policy and no enabled channel the API refuses it with 422 ALERT_SOURCE_NO_ROUTE."),
			"notify_recovery": schema.BoolAttribute{
				Optional:    true,
				Computed:    true,
				Default:     booldefault.StaticBool(true),
				Description: "Whether the people paged hear when the tool's recovery closes the incident.",
			},
			"renotify_interval_secs": schema.Int64Attribute{
				Optional:    true,
				Computed:    true,
				Default:     int64default.StaticInt64(3600),
				Description: "Seconds before the first reminder while an incident stays unacknowledged; each further reminder waits twice as long, up to a day. 0 turns reminders off; otherwise at least 60.",
				Validators:  renotifyValidators(),
			},
			"signing_secret": schema.StringAttribute{
				Optional:  true,
				Sensitive: true,
				Description: "Grafana only: the HMAC secret its contact point signs with, 16 to 256 characters, persisted to Terraform state. On Terraform 1.11+ prefer signing_secret_wo. " +
					"Once set, unsigned requests are refused. Leave it out to set the secret in the app; removing the line keeps the secret, so remove it in the app to stop checking signatures. " +
					"The API never returns the secret: one removed in the app is sent again on the next apply, but one replaced in the app is not detected.",
				Validators: []validator.String{
					secretLength,
					stringvalidator.ConflictsWith(path.MatchRoot("signing_secret_wo")),
				},
			},
			"signing_secret_wo": schema.StringAttribute{
				Optional:    true,
				WriteOnly:   true,
				Sensitive:   true,
				Description: "Write-only signing secret (Terraform 1.11+): sent to the API on apply, never persisted to state or plan. Set signing_secret_wo_version alongside and bump it to rotate.",
				Validators: []validator.String{
					secretLength,
					stringvalidator.AlsoRequires(path.MatchRoot("signing_secret_wo_version")),
				},
			},
			"signing_secret_wo_version": schema.Int64Attribute{
				Optional:    true,
				Description: "Rotation trigger for signing_secret_wo. The secret itself never diffs, so bump this when it changes.",
				Validators:  []validator.Int64{int64validator.AlsoRequires(path.MatchRoot("signing_secret_wo"))},
			},
			"ingest_url": schema.StringAttribute{
				Computed:  true,
				Sensitive: true,
				Description: "Where the tool posts, with the token in the path. A rotation in the app changes it, and the old URL keeps working for 24 hours. " +
					"The API withholds it from a token without `alert_sources:write` and after the server's encryption key was rotated; the last value read is kept then, so refresh with a write token after a rotation.",
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"ingest_token": schema.StringAttribute{
				Computed:      true,
				Sensitive:     true,
				Description:   "The same token, for a tool that sends it as `Authorization: Bearer` to `/hooks/alerts`, which keeps it out of URLs the tool shows in plain text.",
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
		},
	}
}

// ValidateConfig refuses a secret on a kind that does not sign before anything
// is created.
func (r *alertSourceResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var kind types.String
	resp.Diagnostics.Append(req.Config.GetAttribute(ctx, path.Root("kind"), &kind)...)
	if resp.Diagnostics.HasError() || kind.IsUnknown() || kind.IsNull() || kind.ValueString() == client.AlertSourceKindGrafana {
		return
	}
	for _, attr := range []string{"signing_secret", "signing_secret_wo"} {
		var secret types.String
		resp.Diagnostics.Append(req.Config.GetAttribute(ctx, path.Root(attr), &secret)...)
		if !secret.IsNull() {
			resp.Diagnostics.AddAttributeError(path.Root(attr), "Signing secret on a source that does not sign",
				"Only a grafana source signs what it sends; a "+kind.ValueString()+" source takes no signing secret.")
		}
	}
}

func (r *alertSourceResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan, cfg alertSourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}
	plan.SigningSecretWo = cfg.SigningSecretWo
	created, err := r.api.CreateAlertSource(ctx, plan.toNew())
	if err != nil {
		resp.Diagnostics.AddError("Create alert source failed", err.Error())
		return
	}
	settled, err := pauseIfDisabled(ctx, r.api, plan, created)
	if err != nil {
		resp.Diagnostics.AddError("Pause new alert source failed", err.Error())
		if delErr := r.api.DeleteAlertSource(ctx, created.ID); delErr != nil && !client.IsNotFound(delErr) {
			// Saved so Terraform tracks what it made; the error taints it,
			// and nothing posts to its URL yet.
			state := alertSourceToModel(plan, created)
			resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
			resp.Diagnostics.AddError("Remove unpaused alert source failed", delErr.Error())
		}
		return
	}
	state := alertSourceToModel(plan, settled)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// pauseIfDisabled pauses a source the plan wants off, since a create cannot.
func pauseIfDisabled(ctx context.Context, api alertSourceAPI, plan alertSourceModel, created *client.AlertSource) (*client.AlertSource, error) {
	if plan.Enabled.ValueBool() {
		return created, nil
	}
	return api.UpdateAlertSource(ctx, created.ID, client.AlertSourceUpdate{Enabled: new(false)})
}

func (r *alertSourceResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state alertSourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	got, err := r.api.GetAlertSource(ctx, state.ID.ValueString())
	if err != nil {
		if client.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Read alert source failed", err.Error())
		return
	}
	if got.PlanHoldAt != nil {
		resp.Diagnostics.AddWarning("Alert source held by the plan",
			"Alert source "+got.Name+" is held because the plan no longer covers it: new alerts are refused, while open incidents still repeat and close. "+
				"Choose which sources to keep in the app, or move to a plan with more alert sources.")
	}
	if got.RoutesToNobody {
		resp.Diagnostics.AddWarning("Alert source pages nobody",
			"Alert source "+got.Name+" has no live escalation policy of its own, no organization default and no enabled channel, "+
				"so its alerts open incidents that page no one. Bind a policy or an enabled channel.")
	}
	next := alertSourceToModel(state, got)
	next.forgetUnsignedSecret(got)
	resp.Diagnostics.Append(resp.State.Set(ctx, &next)...)
}

func (r *alertSourceResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, prior, cfg alertSourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &prior)...)
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}
	plan.SigningSecretWo = cfg.SigningSecretWo
	updated, err := r.api.UpdateAlertSource(ctx, plan.ID.ValueString(), plan.toUpdate(prior))
	if err != nil {
		resp.Diagnostics.AddError("Update alert source failed", err.Error())
		return
	}
	next := alertSourceToModel(plan, updated)
	resp.Diagnostics.Append(resp.State.Set(ctx, &next)...)
}

func (r *alertSourceResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state alertSourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.api.DeleteAlertSource(ctx, state.ID.ValueString()); err != nil && !client.IsNotFound(err) {
		resp.Diagnostics.AddError("Delete alert source failed", err.Error())
	}
}

func (r *alertSourceResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

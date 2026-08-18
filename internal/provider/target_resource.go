package provider

import (
	"context"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/listvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/setvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64default"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/listdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/mapdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/setdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/setplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/uptimepage/terraform-provider-uptimepage/internal/client"
)

// targetAPI is the minimal client surface the resource needs (interface defined
// by the consumer; *client.Client satisfies it structurally).
type targetAPI interface {
	CreateTarget(ctx context.Context, in client.NewTarget) (*client.Target, error)
	GetTarget(ctx context.Context, id string) (*client.Target, error)
	UpdateTarget(ctx context.Context, id string, in client.TargetUpdate) (*client.Target, error)
	DeleteTarget(ctx context.Context, id string) error
	GetTargetRegions(ctx context.Context, id string) ([]string, error)
	SetTargetRegions(ctx context.Context, id string, regions []string) ([]string, error)
}

var (
	_ resource.Resource                   = (*targetResource)(nil)
	_ resource.ResourceWithConfigure      = (*targetResource)(nil)
	_ resource.ResourceWithImportState    = (*targetResource)(nil)
	_ resource.ResourceWithValidateConfig = (*targetResource)(nil)
)

type targetResource struct {
	api targetAPI
}

func newTargetResource() resource.Resource {
	return &targetResource{}
}

func (r *targetResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_target"
}

func (r *targetResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if c, ok := clientFromProviderData(req.ProviderData, &resp.Diagnostics); ok && c != nil {
		r.api = c
	}
}

func (r *targetResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "A monitored target (uptime check).",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:      true,
				Description:   "Target id (UUID).",
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"name": schema.StringAttribute{
				Required:    true,
				Description: "Human-readable target name.",
			},
			"interval": schema.Int64Attribute{
				Required:    true,
				Description: "Check interval in seconds. The effective minimum is plan- and kind-dependent and enforced server-side: domain_expiry rejects anything under 43200 because RDAP rate-limits by source address, tls_cert under 3600, flow under 300, heartbeat under 60. Expiry checks watch state that moves in days, so 43200 for tls_cert and 86400 for domain_expiry are the usual cadences.",
				Validators:  []validator.Int64{int64validator.AtLeast(10)},
			},
			"enabled": schema.BoolAttribute{
				Optional:    true,
				Computed:    true,
				Default:     booldefault.StaticBool(true),
				Description: "Whether the target is actively checked.",
			},
			"tags": schema.SetAttribute{
				Optional:    true,
				Computed:    true,
				ElementType: types.StringType,
				Default:     setdefault.StaticValue(types.SetValueMust(types.StringType, []attr.Value{})),
				Description: "Free-form tags.",
			},
			"regions": schema.SetAttribute{
				Optional:    true,
				Computed:    true,
				ElementType: types.StringType,
				Description: "Regions this target probes from, as operator-defined slugs (e.g. \"us-east\", \"apac-sg\"). " +
					"Omit to accept whatever the server auto-assigns on create (all regions, up to the plan cap) — that set is read back into state with no perpetual diff. " +
					"Set it to enforce an exact set; the set is replaced wholesale on change. The server requires at least one region and rejects unknown or disabled ids.",
				// No Default: unlike tags (which default to empty), an omitted set
				// is server-computed (the auto-assigned region set), not empty.
				// UseStateForUnknown keeps the prior set in the plan when config is
				// null, so unrelated updates don't churn regions to "known after
				// apply" (a Computed attribute without it plans unknown on update).
				PlanModifiers: []planmodifier.Set{setplanmodifier.UseStateForUnknown()},
				Validators:    []validator.Set{setvalidator.SizeAtLeast(1)},
			},
			"group_name": schema.StringAttribute{
				Optional:    true,
				Description: "Operator-side grouping label.",
			},
			"owner_user_id": schema.StringAttribute{
				Optional:    true,
				Description: "Owning user id (UUID).",
			},
			"alerts": schema.ListNestedAttribute{
				Optional:    true,
				Computed:    true,
				Default:     listdefault.StaticValue(types.ListValueMust(alertObjectType, []attr.Value{})),
				Description: "Alert bindings to notification channels.",
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"channel_id": schema.StringAttribute{
							Required:    true,
							Description: "Notification channel id (UUID).",
						},
						"after_failures": schema.Int64Attribute{
							Required:    true,
							Description: "Consecutive failed checks before alerting (1..1000000).",
							Validators:  []validator.Int64{int64validator.Between(1, 1_000_000)},
						},
						"notify_recovery": schema.BoolAttribute{
							Optional:    true,
							Computed:    true,
							Default:     booldefault.StaticBool(true),
							Description: "Send a recovery notification when the target comes back up.",
						},
					},
				},
			},
			"check": schema.SingleNestedAttribute{
				Required:    true,
				Description: "Check definition. Set `type` and the matching nested block.",
				Attributes: map[string]schema.Attribute{
					"type": schema.StringAttribute{
						Required:    true,
						Description: "Check type: " + strings.Join(checkKinds(), ", ") + ".",
						Validators:  []validator.String{stringvalidator.OneOf(checkKinds()...)},
					},
					"http": schema.SingleNestedAttribute{
						Optional:    true,
						Description: "HTTP(S) check (when type = http).",
						Attributes:  httpCheckAttributes(),
					},
					"tcp":           tcpCheckAttribute(),
					"ping":          pingCheckAttribute(),
					"heartbeat":     heartbeatCheckAttribute(),
					"tls_cert":      tlsCertCheckAttribute(),
					"domain_expiry": domainExpiryCheckAttribute(),
					"dns":           dnsCheckAttribute(),
					"flow":          flowCheckAttribute(),
				},
			},
		},
	}
}

func httpCheckAttributes() map[string]schema.Attribute {
	return map[string]schema.Attribute{
		"url": schema.StringAttribute{
			Required:    true,
			Description: "URL to request.",
		},
		"method": schema.StringAttribute{
			Optional:    true,
			Computed:    true,
			Default:     stringdefault.StaticString("GET"),
			Description: "HTTP method (uppercase).",
			Validators:  []validator.String{stringvalidator.OneOf("GET", "HEAD", "POST", "PUT", "PATCH", "DELETE", "OPTIONS")},
		},
		"timeout_ms": schema.Int64Attribute{
			Optional:    true,
			Computed:    true,
			Default:     int64default.StaticInt64(5000),
			Description: "Request timeout in milliseconds (100..60000).",
			Validators:  []validator.Int64{int64validator.Between(100, 60000)},
		},
		"follow_redirects": schema.BoolAttribute{
			Optional: true,
			Computed: true,
			Default:  booldefault.StaticBool(true),
		},
		"max_redirects": schema.Int64Attribute{
			Optional:   true,
			Computed:   true,
			Default:    int64default.StaticInt64(5),
			Validators: []validator.Int64{int64validator.Between(0, 10)},
		},
		"expected_status": schema.SingleNestedAttribute{
			Required:    true,
			Description: "Expected HTTP status matcher.",
			Attributes: map[string]schema.Attribute{
				"kind": schema.StringAttribute{
					Required:    true,
					Description: "One of: exact, range, one_of.",
					Validators: []validator.String{stringvalidator.OneOf(
						client.StatusKindExact, client.StatusKindRange, client.StatusKindOneOf)},
				},
				"exact": schema.Int64Attribute{
					Optional:    true,
					Description: "Expected status when kind = exact.",
					Validators:  []validator.Int64{int64validator.Between(100, 599)},
				},
				"range": schema.SingleNestedAttribute{
					Optional:    true,
					Description: "Inclusive status range when kind = range.",
					Attributes: map[string]schema.Attribute{
						"min": schema.Int64Attribute{Required: true, Validators: []validator.Int64{int64validator.Between(100, 599)}},
						"max": schema.Int64Attribute{Required: true, Validators: []validator.Int64{int64validator.Between(100, 599)}},
					},
				},
				"one_of": schema.ListAttribute{
					Optional:    true,
					ElementType: types.Int64Type,
					Description: "Accepted statuses when kind = one_of.",
					Validators: []validator.List{
						listvalidator.SizeAtLeast(1),
						listvalidator.ValueInt64sAre(int64validator.Between(100, 599)),
					},
				},
			},
		},
		"expected_body_contains": schema.StringAttribute{
			Optional:    true,
			Description: "Substring the response body must contain.",
		},
		"headers": schema.MapAttribute{
			Optional:    true,
			Computed:    true,
			ElementType: types.StringType,
			Default:     mapdefault.StaticValue(types.MapValueMust(types.StringType, map[string]attr.Value{})),
			Description: "Request headers.",
		},
		"body": schema.StringAttribute{
			Optional:    true,
			Description: "Request body.",
		},
		"verify_tls": schema.BoolAttribute{
			Optional: true,
			Computed: true,
			Default:  booldefault.StaticBool(true),
		},
		"basic_auth": schema.SingleNestedAttribute{
			Optional:    true,
			Sensitive:   true,
			Description: "HTTP basic auth. The API never returns the secret, so external changes to it are not detected.",
			Attributes: map[string]schema.Attribute{
				"username": schema.StringAttribute{Required: true, Sensitive: true},
				"password": schema.StringAttribute{
					Optional:    true,
					Sensitive:   true,
					Description: "Password, persisted to Terraform state. On Terraform 1.11+ prefer password_wo, which never reaches state.",
					Validators: []validator.String{stringvalidator.ConflictsWith(
						path.MatchRelative().AtParent().AtName("password_wo"))},
				},
				"password_wo": schema.StringAttribute{
					Optional:    true,
					WriteOnly:   true,
					Sensitive:   true,
					Description: "Write-only password (Terraform 1.11+): sent to the API on apply, never persisted to state or plan. Set password_wo_version alongside and bump it to rotate.",
					Validators: []validator.String{stringvalidator.AlsoRequires(
						path.MatchRelative().AtParent().AtName("password_wo_version"))},
				},
				"password_wo_version": schema.Int64Attribute{
					Optional:    true,
					Description: "Rotation trigger for password_wo. The password itself never diffs, so bump this when it changes.",
					Validators: []validator.Int64{int64validator.AlsoRequires(
						path.MatchRelative().AtParent().AtName("password_wo"))},
				},
			},
		},
		"bearer_token": schema.StringAttribute{
			Optional:    true,
			Sensitive:   true,
			Description: "Bearer token, persisted to Terraform state. On Terraform 1.11+ prefer bearer_token_wo, which never reaches state. The API never returns the value, so external changes to it are not detected.",
			Validators: []validator.String{stringvalidator.ConflictsWith(
				path.MatchRelative().AtParent().AtName("bearer_token_wo"))},
		},
		"bearer_token_wo": schema.StringAttribute{
			Optional:    true,
			WriteOnly:   true,
			Sensitive:   true,
			Description: "Write-only bearer token (Terraform 1.11+): sent to the API on apply, never persisted to state or plan. Set bearer_token_wo_version alongside and bump it to rotate.",
			Validators: []validator.String{stringvalidator.AlsoRequires(
				path.MatchRelative().AtParent().AtName("bearer_token_wo_version"))},
		},
		"bearer_token_wo_version": schema.Int64Attribute{
			Optional:    true,
			Description: "Rotation trigger for bearer_token_wo. The token itself never diffs, so bump this when it changes.",
			Validators: []validator.Int64{int64validator.AlsoRequires(
				path.MatchRelative().AtParent().AtName("bearer_token_wo"))},
		},
	}
}

func timeoutMsAttribute() schema.Attribute {
	return schema.Int64Attribute{
		Optional:    true,
		Computed:    true,
		Default:     int64default.StaticInt64(5000),
		Description: "Timeout in milliseconds (100..60000).",
		Validators:  []validator.Int64{int64validator.Between(100, 60000)},
	}
}

func portAttribute() schema.Attribute {
	return schema.Int64Attribute{
		Required:    true,
		Description: "Port (1..65535).",
		Validators:  []validator.Int64{int64validator.Between(1, 65535)},
	}
}

// The form clause, appended once so five attributes cannot describe it
// differently. See canonicalHostValidator for why the form is load-bearing.
const hostFormClause = " Lowercase, no trailing dot, punycode for non-ASCII."

func hostAttribute(attr, purpose string) schema.Attribute {
	return schema.StringAttribute{
		Required:    true,
		Description: purpose + hostFormClause,
		Validators:  []validator.String{canonicalHostValidator(attr)},
	}
}

func expiryDaysAttribute(desc string) schema.Attribute {
	return schema.Int64Attribute{
		Required:    true,
		Description: strings.TrimSuffix(desc, ".") + " (1..365).",
		Validators:  []validator.Int64{int64validator.Between(1, 365)},
	}
}

func tcpCheckAttribute() schema.Attribute {
	return schema.SingleNestedAttribute{
		Optional:    true,
		Description: "TCP connect check (when type = tcp).",
		Attributes: map[string]schema.Attribute{
			"host":       hostAttribute("check.host", "Hostname or IP to connect to."),
			"port":       portAttribute(),
			"timeout_ms": timeoutMsAttribute(),
		},
	}
}

// checkKinds is the one list of check types the provider accepts. The schema
// validator and checkBlocksPresent both read it, so a kind added to the schema
// cannot go missing from config validation, where its absence rejects every
// config that names it.
func checkKinds() []string {
	return []string{
		client.CheckTypeHTTP, client.CheckTypeTCP, client.CheckTypePing,
		client.CheckTypeHeartbeat, client.CheckTypeTLSCert, client.CheckTypeDomainExpiry,
		client.CheckTypeDNS, client.CheckTypeFlow,
	}
}

// checkBlocksPresent reports which nested block each kind's config actually set.
func checkBlocksPresent(c checkModel) map[string]bool {
	return map[string]bool{
		client.CheckTypeHTTP:         c.HTTP != nil,
		client.CheckTypeTCP:          c.TCP != nil,
		client.CheckTypePing:         c.Ping != nil,
		client.CheckTypeHeartbeat:    c.Heartbeat != nil,
		client.CheckTypeTLSCert:      c.TLSCert != nil,
		client.CheckTypeDomainExpiry: c.DomainExpiry != nil,
		client.CheckTypeDNS:          c.DNS != nil,
		client.CheckTypeFlow:         c.Flow != nil,
	}
}

// The window is period+grace: a job that has not reported by then opens an
// incident. max_runtime bounds one run instead, for a job that opens a run with
// /start and then hangs.
func heartbeatCheckAttribute() schema.Attribute {
	const day30 = 30 * 24 * 3600 * 1000
	return schema.SingleNestedAttribute{
		Optional:    true,
		Description: "Inbound dead-man's-switch (when type = heartbeat). The job reports in; silence past period_ms + grace_ms opens an incident. Read its ping URL with the uptimepage_heartbeat data source. Nothing is sent to a heartbeat, so the API rejects regions on one.",
		Attributes: map[string]schema.Attribute{
			"period_ms": schema.Int64Attribute{
				Required:    true,
				Description: "Expected reporting cadence in milliseconds (60000..2592000000). Evaluation runs no finer than once a minute, which sets the floor.",
				Validators:  []validator.Int64{int64validator.Between(60_000, day30)},
			},
			"grace_ms": schema.Int64Attribute{
				Required:    true,
				Description: "Allowance past period_ms before the monitor counts as down, in milliseconds (0..2592000000).",
				Validators:  []validator.Int64{int64validator.Between(0, day30)},
			},
			"max_runtime_ms": schema.Int64Attribute{
				Optional:    true,
				Description: "Cap on one run's /start to finish time in milliseconds (60000..2592000000). Omit to leave a run bounded only by period_ms + grace_ms.",
				Validators:  []validator.Int64{int64validator.Between(60_000, day30)},
			},
		},
	}
}

func pingCheckAttribute() schema.Attribute {
	return schema.SingleNestedAttribute{
		Optional:    true,
		Description: "ICMP echo check (when type = ping). No port: an echo request is not addressed to one.",
		Attributes: map[string]schema.Attribute{
			"host":       hostAttribute("check.host", "Hostname or IP to send the echo request to."),
			"timeout_ms": timeoutMsAttribute(),
		},
	}
}

func tlsCertCheckAttribute() schema.Attribute {
	return schema.SingleNestedAttribute{
		Optional:    true,
		Description: "TLS certificate expiry check (when type = tls_cert). warn_days must be greater than critical_days.",
		Attributes: map[string]schema.Attribute{
			"host":          hostAttribute("check.host", "Hostname to open the TLS connection to."),
			"port":          portAttribute(),
			"server_name":   schema.StringAttribute{Optional: true, Description: "SNI to send if different from host."},
			"warn_days":     expiryDaysAttribute("Warn when the cert expires within this many days."),
			"critical_days": expiryDaysAttribute("Fail when the cert expires within this many days."),
			"timeout_ms":    timeoutMsAttribute(),
		},
	}
}

func domainExpiryCheckAttribute() schema.Attribute {
	return schema.SingleNestedAttribute{
		Optional:    true,
		Description: "Domain registration expiry check (when type = domain_expiry). warn_days must be greater than critical_days.",
		Attributes: map[string]schema.Attribute{
			"domain":        hostAttribute("check.domain", "Domain whose registration expiry is read."),
			"warn_days":     expiryDaysAttribute("Warn when the domain expires within this many days."),
			"critical_days": expiryDaysAttribute("Fail when the domain expires within this many days."),
			"timeout_ms":    timeoutMsAttribute(),
		},
	}
}

func dnsCheckAttribute() schema.Attribute {
	return schema.SingleNestedAttribute{
		Optional:    true,
		Description: "DNS resolution check (when type = dns).",
		Attributes: map[string]schema.Attribute{
			"domain": hostAttribute("check.domain", "Name to resolve (FQDN)."),
			"record_type": schema.StringAttribute{
				Required:    true,
				Description: "DNS record type.",
				Validators: []validator.String{stringvalidator.OneOf(
					"A", "AAAA", "CNAME", "MX", "NS", "TXT", "SOA", "PTR", "CAA", "SRV")},
			},
			"resolver":          schema.StringAttribute{Optional: true, Description: "Custom resolver as ip or ip:port."},
			"expected_contains": schema.StringAttribute{Optional: true, Description: "Substring that must appear in an answer."},
			"timeout_ms":        timeoutMsAttribute(),
		},
	}
}

func flowCheckAttribute() schema.Attribute {
	return schema.SingleNestedAttribute{
		Optional:    true,
		Description: "Browser login/transaction flow check (when type = flow). Runs only where a browser engine is available, so its regions clamp to the flow-capable set.",
		Attributes: map[string]schema.Attribute{
			"start_url": schema.StringAttribute{
				Required:    true,
				Description: "URL the browser opens before running the steps.",
			},
			"steps": schema.ListNestedAttribute{
				Required:    true,
				Description: "Ordered browser actions. Include at least one assert_* step so a broken flow fails.",
				Validators:  []validator.List{listvalidator.SizeBetween(1, 30)},
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"op": schema.StringAttribute{
							Required:    true,
							Description: "Action: goto, click, fill, wait_for, assert_text, assert_url.",
							Validators: []validator.String{stringvalidator.OneOf(
								client.FlowOpGoto, client.FlowOpClick, client.FlowOpFill,
								client.FlowOpWaitFor, client.FlowOpAssertText, client.FlowOpAssertURL)},
						},
						"url": schema.StringAttribute{
							Optional:    true,
							Description: "Navigation URL (op = goto).",
						},
						"selector": schema.StringAttribute{
							Optional:    true,
							Description: "CSS selector (op = click, fill, wait_for, or optionally assert_text).",
						},
						"value": schema.StringAttribute{
							Optional:    true,
							Sensitive:   true,
							Description: "Text to fill (op = fill). The API redacts it on read, so external changes are not detected. Reference an org secret as {{name}} for credentials.",
						},
						"contains": schema.StringAttribute{
							Optional:    true,
							Description: "Substring to assert (op = assert_text or assert_url).",
						},
					},
				},
			},
			"timeout_ms": schema.Int64Attribute{
				Optional:    true,
				Computed:    true,
				Default:     int64default.StaticInt64(30000),
				Description: "Whole-flow timeout in milliseconds (1000..120000).",
				Validators:  []validator.Int64{int64validator.Between(1000, 120000)},
			},
			"step_timeout_ms": schema.Int64Attribute{
				Optional:    true,
				Computed:    true,
				Default:     int64default.StaticInt64(5000),
				Description: "Per-step wait for a selector in milliseconds (100..60000).",
				Validators:  []validator.Int64{int64validator.Between(100, 60000)},
			},
			"verify_tls": schema.BoolAttribute{
				Optional: true,
				Computed: true,
				Default:  booldefault.StaticBool(true),
			},
		},
	}
}

func (r *targetResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan, cfg targetModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}
	graftWriteOnlySecrets(&plan, cfg)

	in, d := plan.toNew(ctx)
	resp.Diagnostics.Append(d...)
	if resp.Diagnostics.HasError() {
		return
	}

	created, err := r.api.CreateTarget(ctx, in)
	if err != nil {
		resp.Diagnostics.AddError("Create target failed", err.Error())
		return
	}

	// prior = plan so in-state secrets and rotation counters survive the
	// redacted read-back.
	state, d := targetToModel(ctx, plan, created)
	resp.Diagnostics.Append(d...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Regions are a sub-resource. If the user configured a set, enforce it;
	// otherwise read back the set the server auto-assigned on create.
	desired := plan.regions(ctx, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	var applied []string
	if desired != nil {
		applied, err = r.api.SetTargetRegions(ctx, created.ID, desired)
		if err != nil {
			resp.Diagnostics.AddError("Set target regions failed", err.Error())
			// The PUT failed (e.g. an invalid region id) but the target exists
			// with its auto-assigned set. Persist it with that set, if readable,
			// so Terraform tracks it instead of leaking an untracked target.
			if cur, gerr := r.api.GetTargetRegions(ctx, created.ID); gerr == nil {
				state.Regions = regionsToSet(ctx, cur, &resp.Diagnostics)
				resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
			}
			return
		}
	} else {
		applied, err = r.api.GetTargetRegions(ctx, created.ID)
		if err != nil {
			resp.Diagnostics.AddError("Read target regions failed", err.Error())
			return
		}
	}
	state.Regions = regionsToSet(ctx, applied, &resp.Diagnostics)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *targetResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state targetModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	got, err := r.api.GetTarget(ctx, state.ID.ValueString())
	if err != nil {
		if client.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Read target failed", err.Error())
		return
	}

	next, d := targetToModel(ctx, state, got)
	resp.Diagnostics.Append(d...)
	if resp.Diagnostics.HasError() {
		return
	}

	regions, err := r.api.GetTargetRegions(ctx, state.ID.ValueString())
	if err != nil {
		if client.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Read target regions failed", err.Error())
		return
	}
	next.Regions = regionsToSet(ctx, regions, &resp.Diagnostics)
	resp.Diagnostics.Append(resp.State.Set(ctx, &next)...)
}

func (r *targetResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, prior, cfg targetModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &prior)...)
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}
	graftWriteOnlySecrets(&plan, cfg)

	in, d := plan.toUpdate(ctx)
	resp.Diagnostics.Append(d...)
	if resp.Diagnostics.HasError() {
		return
	}

	updated, err := r.api.UpdateTarget(ctx, plan.ID.ValueString(), in)
	if err != nil {
		resp.Diagnostics.AddError("Update target failed", err.Error())
		return
	}

	state, d := targetToModel(ctx, plan, updated)
	resp.Diagnostics.Append(d...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Reconcile the region sub-resource only when the desired set changed. When
	// the user omits regions, Optional+Computed carries the prior set into the
	// plan, so this compares equal and no PUT is issued.
	applied := prior.regions(ctx, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	if !plan.Regions.Equal(prior.Regions) {
		if desired := plan.regions(ctx, &resp.Diagnostics); desired != nil {
			applied, err = r.api.SetTargetRegions(ctx, plan.ID.ValueString(), desired)
		} else {
			// Planned set is unknown (e.g. derived from another resource); pull
			// the current set so state stays accurate.
			applied, err = r.api.GetTargetRegions(ctx, plan.ID.ValueString())
		}
		if resp.Diagnostics.HasError() {
			return
		}
		if err != nil {
			resp.Diagnostics.AddError("Set target regions failed", err.Error())
			return
		}
	}
	state.Regions = regionsToSet(ctx, applied, &resp.Diagnostics)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *targetResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state targetModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.api.DeleteTarget(ctx, state.ID.ValueString()); err != nil && !client.IsNotFound(err) {
		resp.Diagnostics.AddError("Delete target failed", err.Error())
	}
}

func (r *targetResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

// graftWriteOnlySecrets copies write-only secret values from the decoded
// config into the planned model: Terraform carries them only in config, never
// in the plan.
func graftWriteOnlySecrets(plan *targetModel, cfg targetModel) {
	if plan.Check.HTTP == nil || cfg.Check.HTTP == nil {
		return
	}
	plan.Check.HTTP.BearerTokenWo = cfg.Check.HTTP.BearerTokenWo
	if plan.Check.HTTP.BasicAuth != nil && cfg.Check.HTTP.BasicAuth != nil {
		plan.Check.HTTP.BasicAuth.PasswordWo = cfg.Check.HTTP.BasicAuth.PasswordWo
	}
}

// ValidateConfig moves the API's cross-field rules to plan time, so a config
// the server would refuse fails before anything is created.
func (r *targetResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var cfg targetModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}
	validateTargetConfig(cfg, &resp.Diagnostics)
}

// Split from the framework plumbing so the rules can be exercised against a
// model rather than a hand-built tfsdk.Config.
func validateTargetConfig(cfg targetModel, diags *diag.Diagnostics) {
	// Null/unknown type: let the framework's Required validator own that error.
	if cfg.Check.Type.IsUnknown() || cfg.Check.Type.IsNull() {
		return
	}
	validateDiscriminatedBlock(path.Root("check"), cfg.Check.Type.ValueString(),
		checkBlocksPresent(cfg.Check), diags)

	if cfg.Check.Type.ValueString() == client.CheckTypeFlow && cfg.Check.Flow != nil {
		validateFlowSteps(cfg.Check.Flow.Steps, diags)
	}

	// Keyed on the declared kind, not on which block happens to be set: a
	// stray block of another kind is already the discriminator's error, and
	// checking it again here would pile a second, unrelated one on top.
	switch cfg.Check.Type.ValueString() {
	case client.CheckTypeHeartbeat:
		if cfg.Check.Heartbeat != nil {
			validateHeartbeatCadence(cfg.Interval, cfg.Check.Heartbeat, diags)
		}
		// The API 422s the regions PUT for a passive check. Left to apply, the
		// target is created first and only then refused its regions.
		if !cfg.Regions.IsNull() && !cfg.Regions.IsUnknown() {
			diags.AddAttributeError(path.Root("regions"),
				"A heartbeat is not probed from regions",
				"Nothing is sent to a heartbeat, so the API rejects regions on one. Remove "+
					"the regions argument.")
		}
	case client.CheckTypeTLSCert:
		if cfg.Check.TLSCert != nil {
			validateExpiryDays(path.Root("check").AtName("tls_cert"),
				cfg.Check.TLSCert.WarnDays, cfg.Check.TLSCert.CriticalDays, diags)
		}
	case client.CheckTypeDomainExpiry:
		if cfg.Check.DomainExpiry != nil {
			validateExpiryDays(path.Root("check").AtName("domain_expiry"),
				cfg.Check.DomainExpiry.WarnDays, cfg.Check.DomainExpiry.CriticalDays, diags)
		}
	}

	// ConflictsWith rules out both; this rules out neither.
	if cfg.Check.HTTP != nil && cfg.Check.HTTP.BasicAuth != nil {
		ba := cfg.Check.HTTP.BasicAuth
		if ba.Password.IsNull() && ba.PasswordWo.IsNull() {
			diags.AddAttributeError(
				path.Root("check").AtName("http").AtName("basic_auth"),
				"Missing basic auth password",
				"Set either password (persisted to Terraform state) or password_wo with password_wo_version (write-only, Terraform 1.11+).",
			)
		}
	}
}

// validateHeartbeatCadence enforces that the check interval can actually see
// the window it judges. An interval coarser than period+grace steps straight
// over the deadline, so the monitor reports late or not at all.
func validateHeartbeatCadence(interval types.Int64, hb *heartbeatCheckModel, diags *diag.Diagnostics) {
	if interval.IsNull() || interval.IsUnknown() ||
		hb.PeriodMs.IsNull() || hb.PeriodMs.IsUnknown() ||
		hb.GraceMs.IsNull() || hb.GraceMs.IsUnknown() {
		return
	}
	// Evaluation runs once a minute, so a tighter interval buys nothing and
	// the API refuses it.
	if interval.ValueInt64() < 60 {
		diags.AddAttributeError(path.Root("interval"),
			"Check interval is below the heartbeat floor",
			fmt.Sprintf("interval is %ds, but a heartbeat is evaluated no more than once a "+
				"minute, so the API rejects anything under 60.", interval.ValueInt64()))
		return
	}
	// The API compares whole seconds, so match its truncation rather than
	// rejecting a config it would accept.
	window := hb.PeriodMs.ValueInt64()/1000 + hb.GraceMs.ValueInt64()/1000
	if interval.ValueInt64() > window {
		diags.AddAttributeError(path.Root("interval"),
			"Check interval is longer than the heartbeat window",
			fmt.Sprintf("interval is %ds but period_ms + grace_ms is only %ds, so the deadline "+
				"can pass unseen. Lower the interval or raise period_ms.",
				interval.ValueInt64(), window))
	}
}

// validateExpiryDays enforces the ordering the API requires. Equal days is the
// one that looks fine and is not: the warning can never fire before the
// failure, so the check only ever reports critical.
func validateExpiryDays(at path.Path, warn, critical types.Int64, diags *diag.Diagnostics) {
	if warn.IsNull() || warn.IsUnknown() || critical.IsNull() || critical.IsUnknown() {
		return
	}
	if warn.ValueInt64() <= critical.ValueInt64() {
		diags.AddAttributeError(at.AtName("warn_days"),
			"warn_days must be greater than critical_days",
			fmt.Sprintf("warn_days is %d and critical_days is %d, so the warning would never "+
				"fire before the failure. The API rejects this.",
				warn.ValueInt64(), critical.ValueInt64()))
	}
}

// validateFlowSteps enforces that each step carries the fields its op needs, so
// an omitted field is a plan-time error rather than an empty string sent to the
// server (which for a shared field would also perpetually diff against the null
// config). Unknown values are left for apply-time, when they resolve.
func validateFlowSteps(steps []flowStepModel, diags *diag.Diagnostics) {
	stepsPath := path.Root("check").AtName("flow").AtName("steps")
	for i, s := range steps {
		if s.Op.IsUnknown() || s.Op.IsNull() {
			continue
		}
		op := s.Op.ValueString()
		at := stepsPath.AtListIndex(i)
		need := func(field string, v types.String, allowEmpty bool) {
			if v.IsUnknown() {
				return
			}
			if v.IsNull() || (!allowEmpty && v.ValueString() == "") {
				diags.AddAttributeError(at.AtName(field), "Missing flow step field",
					fmt.Sprintf("op = %q requires %q.", op, field))
			}
		}
		switch op {
		case client.FlowOpGoto:
			need("url", s.URL, false)
		case client.FlowOpClick, client.FlowOpWaitFor:
			need("selector", s.Selector, false)
		case client.FlowOpFill:
			need("selector", s.Selector, false)
			need("value", s.Value, true) // an explicit empty fill is allowed
		case client.FlowOpAssertText, client.FlowOpAssertURL:
			need("contains", s.Contains, false)
		}
	}
}

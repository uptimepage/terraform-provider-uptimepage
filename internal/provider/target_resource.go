package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/listvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64default"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/listdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/mapdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/setdefault"
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
				Description: "Check interval in seconds (the effective minimum is plan-dependent and enforced server-side).",
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
						Description: "Check type: http, tcp, tls_cert, domain_expiry, dns.",
						Validators: []validator.String{stringvalidator.OneOf(
							client.CheckTypeHTTP, client.CheckTypeTCP, client.CheckTypeTLSCert,
							client.CheckTypeDomainExpiry, client.CheckTypeDNS)},
					},
					"http": schema.SingleNestedAttribute{
						Optional:    true,
						Description: "HTTP(S) check (when type = http).",
						Attributes:  httpCheckAttributes(),
					},
					"tcp":           tcpCheckAttribute(),
					"tls_cert":      tlsCertCheckAttribute(),
					"domain_expiry": domainExpiryCheckAttribute(),
					"dns":           dnsCheckAttribute(),
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
			Description: "HTTP basic auth. Write-only: the API never returns the value, so external changes to it are not detected.",
			Attributes: map[string]schema.Attribute{
				"username": schema.StringAttribute{Required: true, Sensitive: true},
				"password": schema.StringAttribute{Required: true, Sensitive: true},
			},
		},
		"bearer_token": schema.StringAttribute{
			Optional:    true,
			Sensitive:   true,
			Description: "Bearer token. Write-only: the API never returns the value, so external changes to it are not detected.",
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

func expiryDaysAttribute(desc string) schema.Attribute {
	return schema.Int64Attribute{
		Required:    true,
		Description: desc,
		Validators:  []validator.Int64{int64validator.Between(0, 36500)},
	}
}

func tcpCheckAttribute() schema.Attribute {
	return schema.SingleNestedAttribute{
		Optional:    true,
		Description: "TCP connect check (when type = tcp).",
		Attributes: map[string]schema.Attribute{
			"host":       schema.StringAttribute{Required: true},
			"port":       portAttribute(),
			"timeout_ms": timeoutMsAttribute(),
		},
	}
}

func tlsCertCheckAttribute() schema.Attribute {
	return schema.SingleNestedAttribute{
		Optional:    true,
		Description: "TLS certificate expiry check (when type = tls_cert).",
		Attributes: map[string]schema.Attribute{
			"host":          schema.StringAttribute{Required: true},
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
		Description: "Domain registration expiry check (when type = domain_expiry).",
		Attributes: map[string]schema.Attribute{
			"domain":        schema.StringAttribute{Required: true},
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
			"domain": schema.StringAttribute{Required: true, Description: "Name to resolve (FQDN)."},
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

func (r *targetResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan targetModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

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

	// prior = plan so write-only secrets survive the redacted read-back.
	state, d := targetToModel(ctx, plan, created)
	resp.Diagnostics.Append(d...)
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
	resp.Diagnostics.Append(resp.State.Set(ctx, &next)...)
}

func (r *targetResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan targetModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

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

// ValidateConfig enforces that exactly the nested block matching check.type is
// set, surfaced at plan time rather than as an apply-time API error.
func (r *targetResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var cfg targetModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	// Null/unknown type: let the framework's Required validator own that error.
	if resp.Diagnostics.HasError() || cfg.Check.Type.IsUnknown() || cfg.Check.Type.IsNull() {
		return
	}
	validateDiscriminatedBlock(path.Root("check"), cfg.Check.Type.ValueString(), map[string]bool{
		client.CheckTypeHTTP:         cfg.Check.HTTP != nil,
		client.CheckTypeTCP:          cfg.Check.TCP != nil,
		client.CheckTypeTLSCert:      cfg.Check.TLSCert != nil,
		client.CheckTypeDomainExpiry: cfg.Check.DomainExpiry != nil,
		client.CheckTypeDNS:          cfg.Check.DNS != nil,
	}, &resp.Diagnostics)
}

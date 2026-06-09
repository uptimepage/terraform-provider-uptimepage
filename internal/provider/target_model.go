package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/uptimepage/terraform-provider-uptimepage/internal/client"
)

// redactedSentinel is what the API returns for write-only secret fields on read.
const redactedSentinel = "***"

// targetModel is the tfsdk view of an uptimepage_target.
type targetModel struct {
	ID          types.String `tfsdk:"id"`
	Name        types.String `tfsdk:"name"`
	Interval    types.Int64  `tfsdk:"interval"`
	Enabled     types.Bool   `tfsdk:"enabled"`
	Tags        types.Set    `tfsdk:"tags"`
	Regions     types.Set    `tfsdk:"regions"`
	GroupName   types.String `tfsdk:"group_name"`
	OwnerUserID types.String `tfsdk:"owner_user_id"`
	Alerts      []alertModel `tfsdk:"alerts"`
	Check       checkModel   `tfsdk:"check"`
}

type alertModel struct {
	ChannelID      types.String `tfsdk:"channel_id"`
	AfterFailures  types.Int64  `tfsdk:"after_failures"`
	NotifyRecovery types.Bool   `tfsdk:"notify_recovery"`
}

// checkModel is the discriminated check block: Type names the active variant
// and exactly one of the per-type pointers is set.
type checkModel struct {
	Type         types.String       `tfsdk:"type"`
	HTTP         *httpCheckModel    `tfsdk:"http"`
	TCP          *tcpCheckModel     `tfsdk:"tcp"`
	TLSCert      *tlsCertCheckModel `tfsdk:"tls_cert"`
	DomainExpiry *domainExpiryModel `tfsdk:"domain_expiry"`
	DNS          *dnsCheckModel     `tfsdk:"dns"`
}

type httpCheckModel struct {
	URL                  types.String        `tfsdk:"url"`
	Method               types.String        `tfsdk:"method"`
	TimeoutMs            types.Int64         `tfsdk:"timeout_ms"`
	FollowRedirects      types.Bool          `tfsdk:"follow_redirects"`
	MaxRedirects         types.Int64         `tfsdk:"max_redirects"`
	ExpectedStatus       expectedStatusModel `tfsdk:"expected_status"`
	ExpectedBodyContains types.String        `tfsdk:"expected_body_contains"`
	Headers              types.Map           `tfsdk:"headers"`
	Body                 types.String        `tfsdk:"body"`
	VerifyTLS            types.Bool          `tfsdk:"verify_tls"`
	BasicAuth            *basicAuthModel     `tfsdk:"basic_auth"`
	BearerToken          types.String        `tfsdk:"bearer_token"`
}

type tcpCheckModel struct {
	Host      types.String `tfsdk:"host"`
	Port      types.Int64  `tfsdk:"port"`
	TimeoutMs types.Int64  `tfsdk:"timeout_ms"`
}

type tlsCertCheckModel struct {
	Host         types.String `tfsdk:"host"`
	Port         types.Int64  `tfsdk:"port"`
	ServerName   types.String `tfsdk:"server_name"`
	WarnDays     types.Int64  `tfsdk:"warn_days"`
	CriticalDays types.Int64  `tfsdk:"critical_days"`
	TimeoutMs    types.Int64  `tfsdk:"timeout_ms"`
}

type domainExpiryModel struct {
	Domain       types.String `tfsdk:"domain"`
	WarnDays     types.Int64  `tfsdk:"warn_days"`
	CriticalDays types.Int64  `tfsdk:"critical_days"`
	TimeoutMs    types.Int64  `tfsdk:"timeout_ms"`
}

type dnsCheckModel struct {
	Domain           types.String `tfsdk:"domain"`
	RecordType       types.String `tfsdk:"record_type"`
	Resolver         types.String `tfsdk:"resolver"`
	ExpectedContains types.String `tfsdk:"expected_contains"`
	TimeoutMs        types.Int64  `tfsdk:"timeout_ms"`
}

type expectedStatusModel struct {
	Kind  types.String `tfsdk:"kind"`
	Exact types.Int64  `tfsdk:"exact"`
	Range *rangeModel  `tfsdk:"range"`
	OneOf types.List   `tfsdk:"one_of"`
}

type rangeModel struct {
	Min types.Int64 `tfsdk:"min"`
	Max types.Int64 `tfsdk:"max"`
}

type basicAuthModel struct {
	Username types.String `tfsdk:"username"`
	Password types.String `tfsdk:"password"`
}

// --- model -> wire ---

func (m targetModel) toNew(ctx context.Context) (client.NewTarget, diag.Diagnostics) {
	var diags diag.Diagnostics
	check, cd := m.Check.toWire(ctx)
	diags.Append(cd...)

	out := client.NewTarget{
		Name:        m.Name.ValueString(),
		Check:       check,
		Interval:    uint64(m.Interval.ValueInt64()),
		Enabled:     m.Enabled.ValueBool(),
		Tags:        m.tags(ctx, &diags),
		Alerts:      m.alerts(),
		GroupName:   optString(m.GroupName),
		OwnerUserID: optString(m.OwnerUserID),
	}
	return out, diags
}

func (m targetModel) toUpdate(ctx context.Context) (client.TargetUpdate, diag.Diagnostics) {
	var diags diag.Diagnostics
	check, cd := m.Check.toWire(ctx)
	diags.Append(cd...)

	out := client.TargetUpdate{
		Name:        m.Name.ValueString(),
		Check:       check,
		Interval:    uint64(m.Interval.ValueInt64()),
		Enabled:     m.Enabled.ValueBool(),
		Tags:        m.tags(ctx, &diags),
		Alerts:      m.alerts(),
		GroupName:   optString(m.GroupName),
		OwnerUserID: optString(m.OwnerUserID),
	}
	return out, diags
}

func (m targetModel) tags(ctx context.Context, diags *diag.Diagnostics) []string {
	if m.Tags.IsNull() || m.Tags.IsUnknown() {
		return nil
	}
	var tags []string
	diags.Append(m.Tags.ElementsAs(ctx, &tags, false)...)
	return tags
}

// regions extracts the configured region set as a plain slice. Regions are a
// separate sub-resource (not part of the target create/update body), so this is
// used by the resource CRUD to PUT the set, not by toNew/toUpdate. A null or
// unknown set returns nil, meaning "leave the server-assigned set in place".
func (m targetModel) regions(ctx context.Context, diags *diag.Diagnostics) []string {
	if m.Regions.IsNull() || m.Regions.IsUnknown() {
		return nil
	}
	var regions []string
	diags.Append(m.Regions.ElementsAs(ctx, &regions, false)...)
	return regions
}

// regionsToSet converts a region slice read back from the API into a tfsdk Set.
// A Set is order-insensitive, so the server's ordering never produces a diff.
func regionsToSet(ctx context.Context, regions []string, diags *diag.Diagnostics) types.Set {
	set, d := types.SetValueFrom(ctx, types.StringType, regions)
	diags.Append(d...)
	return set
}

func (m targetModel) alerts() []client.AlertBinding {
	out := make([]client.AlertBinding, 0, len(m.Alerts))
	for _, a := range m.Alerts {
		out = append(out, client.AlertBinding{
			ChannelID:      a.ChannelID.ValueString(),
			AfterFailures:  uint32(a.AfterFailures.ValueInt64()),
			NotifyRecovery: a.NotifyRecovery.ValueBool(),
		})
	}
	return out
}

func (c checkModel) toWire(ctx context.Context) (client.CheckSpec, diag.Diagnostics) {
	var diags diag.Diagnostics
	kind := c.Type.ValueString()
	out := client.CheckSpec{Type: kind}

	switch kind {
	case client.CheckTypeHTTP:
		if c.HTTP == nil {
			return out, missingBlock(kind)
		}
		out.HTTP, diags = c.HTTP.toWire(ctx)
	case client.CheckTypeTCP:
		if c.TCP == nil {
			return out, missingBlock(kind)
		}
		out.TCP = &client.TCPCheck{
			Host:    c.TCP.Host.ValueString(),
			Port:    uint16(c.TCP.Port.ValueInt64()),
			Timeout: uint64(c.TCP.TimeoutMs.ValueInt64()),
		}
	case client.CheckTypeTLSCert:
		if c.TLSCert == nil {
			return out, missingBlock(kind)
		}
		out.TLSCert = &client.TLSCertCheck{
			Host:         c.TLSCert.Host.ValueString(),
			Port:         uint16(c.TLSCert.Port.ValueInt64()),
			ServerName:   optString(c.TLSCert.ServerName),
			WarnDays:     uint32(c.TLSCert.WarnDays.ValueInt64()),
			CriticalDays: uint32(c.TLSCert.CriticalDays.ValueInt64()),
			Timeout:      uint64(c.TLSCert.TimeoutMs.ValueInt64()),
		}
	case client.CheckTypeDomainExpiry:
		if c.DomainExpiry == nil {
			return out, missingBlock(kind)
		}
		out.DomainExpiry = &client.DomainExpiryCheck{
			Domain:       c.DomainExpiry.Domain.ValueString(),
			WarnDays:     uint32(c.DomainExpiry.WarnDays.ValueInt64()),
			CriticalDays: uint32(c.DomainExpiry.CriticalDays.ValueInt64()),
			Timeout:      uint64(c.DomainExpiry.TimeoutMs.ValueInt64()),
		}
	case client.CheckTypeDNS:
		if c.DNS == nil {
			return out, missingBlock(kind)
		}
		out.DNS = &client.DNSCheck{
			Domain:           c.DNS.Domain.ValueString(),
			RecordType:       c.DNS.RecordType.ValueString(),
			Resolver:         optString(c.DNS.Resolver),
			ExpectedContains: optString(c.DNS.ExpectedContains),
			Timeout:          uint64(c.DNS.TimeoutMs.ValueInt64()),
		}
	default:
		diags.AddError("Invalid check", fmt.Sprintf("unsupported check type %q", kind))
	}
	return out, diags
}

func (h httpCheckModel) toWire(ctx context.Context) (*client.HTTPCheck, diag.Diagnostics) {
	es, diags := h.ExpectedStatus.toWire(ctx)
	out := &client.HTTPCheck{
		URL:                  h.URL.ValueString(),
		Method:               h.Method.ValueString(),
		Timeout:              uint64(h.TimeoutMs.ValueInt64()),
		FollowRedirects:      h.FollowRedirects.ValueBool(),
		MaxRedirects:         uint8(h.MaxRedirects.ValueInt64()),
		ExpectedStatus:       es,
		ExpectedBodyContains: optString(h.ExpectedBodyContains),
		Headers:              mapToStrings(ctx, h.Headers, &diags),
		Body:                 optString(h.Body),
		VerifyTLS:            h.VerifyTLS.ValueBool(),
		BearerToken:          optString(h.BearerToken),
	}
	if h.BasicAuth != nil {
		out.BasicAuth = &[2]string{h.BasicAuth.Username.ValueString(), h.BasicAuth.Password.ValueString()}
	}
	return out, diags
}

func missingBlock(kind string) diag.Diagnostics {
	var d diag.Diagnostics
	d.AddError("Invalid check", fmt.Sprintf("type = %q requires the %q block.", kind, kind))
	return d
}

func (e expectedStatusModel) toWire(ctx context.Context) (client.ExpectedStatus, diag.Diagnostics) {
	var diags diag.Diagnostics
	out := client.ExpectedStatus{Kind: e.Kind.ValueString()}
	switch out.Kind {
	case client.StatusKindExact:
		if e.Exact.IsNull() || e.Exact.IsUnknown() {
			diags.AddError("Invalid expected_status", `kind = "exact" requires "exact" to be set.`)
			return out, diags
		}
		out.Exact = uint16(e.Exact.ValueInt64())
	case client.StatusKindRange:
		if e.Range == nil {
			diags.AddError("Invalid expected_status", `kind = "range" requires the "range" block.`)
			return out, diags
		}
		out.Range = &client.StatusRange{
			Min: uint16(e.Range.Min.ValueInt64()),
			Max: uint16(e.Range.Max.ValueInt64()),
		}
	case client.StatusKindOneOf:
		var codes []int64
		diags.Append(e.OneOf.ElementsAs(ctx, &codes, false)...)
		if len(codes) == 0 {
			diags.AddError("Invalid expected_status", `kind = "one_of" requires a non-empty "one_of" list.`)
			return out, diags
		}
		out.OneOf = make([]uint16, len(codes))
		for i, c := range codes {
			out.OneOf[i] = uint16(c)
		}
	}
	return out, diags
}

// --- wire -> model ---

// targetToModel maps a read Target into the tfsdk model. prior carries the
// pre-existing state so write-only secrets (basic_auth/bearer_token), which the
// API returns redacted, keep their known values instead of showing a diff.
func targetToModel(ctx context.Context, prior targetModel, t *client.Target) (targetModel, diag.Diagnostics) {
	var diags diag.Diagnostics

	tags, d := types.SetValueFrom(ctx, types.StringType, t.Tags)
	diags.Append(d...)

	m := targetModel{
		ID:          types.StringValue(t.ID),
		Name:        types.StringValue(t.Name),
		Interval:    types.Int64Value(int64(t.Interval)),
		Enabled:     types.BoolValue(t.Enabled),
		Tags:        tags,
		GroupName:   fromOptString(t.GroupName),
		OwnerUserID: fromOptString(t.OwnerUserID),
		Alerts:      alertsToModel(t.Alerts),
	}

	check, cd := checkToModel(ctx, prior.Check, t.Check)
	diags.Append(cd...)
	m.Check = check
	return m, diags
}

func alertsToModel(in []client.AlertBinding) []alertModel {
	// Non-nil empty so the read-back matches an Optional+Computed default of an
	// empty list (nil would map to a null list and diff forever).
	out := make([]alertModel, 0, len(in))
	for _, a := range in {
		out = append(out, alertModel{
			ChannelID:      types.StringValue(a.ChannelID),
			AfterFailures:  types.Int64Value(int64(a.AfterFailures)),
			NotifyRecovery: types.BoolValue(a.NotifyRecovery),
		})
	}
	return out
}

func checkToModel(ctx context.Context, prior checkModel, spec client.CheckSpec) (checkModel, diag.Diagnostics) {
	var diags diag.Diagnostics
	out := checkModel{Type: types.StringValue(spec.Type)}

	switch {
	case spec.HTTP != nil:
		out.HTTP, diags = httpToModel(ctx, prior.HTTP, spec.HTTP)
	case spec.TCP != nil:
		out.TCP = &tcpCheckModel{
			Host:      types.StringValue(spec.TCP.Host),
			Port:      types.Int64Value(int64(spec.TCP.Port)),
			TimeoutMs: types.Int64Value(int64(spec.TCP.Timeout)),
		}
	case spec.TLSCert != nil:
		out.TLSCert = &tlsCertCheckModel{
			Host:         types.StringValue(spec.TLSCert.Host),
			Port:         types.Int64Value(int64(spec.TLSCert.Port)),
			ServerName:   fromOptString(spec.TLSCert.ServerName),
			WarnDays:     types.Int64Value(int64(spec.TLSCert.WarnDays)),
			CriticalDays: types.Int64Value(int64(spec.TLSCert.CriticalDays)),
			TimeoutMs:    types.Int64Value(int64(spec.TLSCert.Timeout)),
		}
	case spec.DomainExpiry != nil:
		out.DomainExpiry = &domainExpiryModel{
			Domain:       types.StringValue(spec.DomainExpiry.Domain),
			WarnDays:     types.Int64Value(int64(spec.DomainExpiry.WarnDays)),
			CriticalDays: types.Int64Value(int64(spec.DomainExpiry.CriticalDays)),
			TimeoutMs:    types.Int64Value(int64(spec.DomainExpiry.Timeout)),
		}
	case spec.DNS != nil:
		out.DNS = &dnsCheckModel{
			Domain:           types.StringValue(spec.DNS.Domain),
			RecordType:       types.StringValue(spec.DNS.RecordType),
			Resolver:         fromOptString(spec.DNS.Resolver),
			ExpectedContains: fromOptString(spec.DNS.ExpectedContains),
			TimeoutMs:        types.Int64Value(int64(spec.DNS.Timeout)),
		}
	default:
		diags.AddError("Unsupported check type", fmt.Sprintf("check type %q has no payload", spec.Type))
	}
	return out, diags
}

func httpToModel(ctx context.Context, prior *httpCheckModel, h *client.HTTPCheck) (*httpCheckModel, diag.Diagnostics) {
	headers, diags := types.MapValueFrom(ctx, types.StringType, h.Headers)

	es, esd := expectedStatusToModel(ctx, h.ExpectedStatus)
	diags.Append(esd...)

	var priorBasic *basicAuthModel
	priorBearer := types.StringNull()
	priorURL := types.StringNull()
	if prior != nil {
		priorBasic = prior.BasicAuth
		priorBearer = prior.BearerToken
		priorURL = prior.URL
	}

	return &httpCheckModel{
		// API canonicalizes the URL; keep the user's form when equivalent.
		URL:                  keepURL(priorURL, h.URL),
		Method:               types.StringValue(h.Method),
		TimeoutMs:            types.Int64Value(int64(h.Timeout)),
		FollowRedirects:      types.BoolValue(h.FollowRedirects),
		MaxRedirects:         types.Int64Value(int64(h.MaxRedirects)),
		ExpectedStatus:       es,
		ExpectedBodyContains: fromOptString(h.ExpectedBodyContains),
		Headers:              headers,
		Body:                 fromOptString(h.Body),
		VerifyTLS:            types.BoolValue(h.VerifyTLS),
		// Secrets: the API redacts these on read, so trust prior state.
		BasicAuth:   keepBasicAuth(priorBasic, h.BasicAuth),
		BearerToken: keepSecret(priorBearer, h.BearerToken),
	}, diags
}

func expectedStatusToModel(ctx context.Context, e client.ExpectedStatus) (expectedStatusModel, diag.Diagnostics) {
	var diags diag.Diagnostics
	out := expectedStatusModel{
		Kind:  types.StringValue(e.Kind),
		Exact: types.Int64Null(),
		OneOf: types.ListNull(types.Int64Type),
	}
	switch e.Kind {
	case client.StatusKindExact:
		out.Exact = types.Int64Value(int64(e.Exact))
	case client.StatusKindRange:
		if e.Range != nil {
			out.Range = &rangeModel{
				Min: types.Int64Value(int64(e.Range.Min)),
				Max: types.Int64Value(int64(e.Range.Max)),
			}
		}
	case client.StatusKindOneOf:
		codes := make([]int64, len(e.OneOf))
		for i, c := range e.OneOf {
			codes[i] = int64(c)
		}
		list, d := types.ListValueFrom(ctx, types.Int64Type, codes)
		diags.Append(d...)
		out.OneOf = list
	}
	return out, diags
}

// keepBasicAuth returns the prior value when the API echoes the redaction
// sentinel; otherwise it reflects what the API returned (e.g. cleared).
func keepBasicAuth(prior *basicAuthModel, got *[2]string) *basicAuthModel {
	if got != nil && got[0] == redactedSentinel {
		return prior
	}
	if got == nil {
		return nil
	}
	return &basicAuthModel{
		Username: types.StringValue(got[0]),
		Password: types.StringValue(got[1]),
	}
}

func keepSecret(prior types.String, got *string) types.String {
	if got != nil && *got == redactedSentinel {
		return prior
	}
	return fromOptString(got)
}

// --- small helpers ---

func optString(s types.String) *string {
	if s.IsNull() || s.IsUnknown() {
		return nil
	}
	v := s.ValueString()
	return &v
}

func fromOptString(s *string) types.String {
	if s == nil {
		return types.StringNull()
	}
	return types.StringValue(*s)
}

func mapToStrings(ctx context.Context, m types.Map, diags *diag.Diagnostics) map[string]string {
	if m.IsNull() || m.IsUnknown() {
		return map[string]string{}
	}
	out := map[string]string{}
	diags.Append(m.ElementsAs(ctx, &out, false)...)
	return out
}

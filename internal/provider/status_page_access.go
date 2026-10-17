package provider

import (
	"context"
	"fmt"
	"maps"
	"strings"
	"unicode"

	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/setvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/objectdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/setdefault"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/uptimepage/terraform-provider-uptimepage/internal/client"
)

const (
	passkeysByDefault = true
	emailByDefault    = true
)

var pageEmailAccessAttrTypes = map[string]attr.Type{
	"enabled":          types.BoolType,
	"passkeys_enabled": types.BoolType,
	"recheck_days":     types.Int64Type,
}

var pagePasswordAccessAttrTypes = map[string]attr.Type{
	"enabled":          types.BoolType,
	"value_wo":         types.StringType,
	"value_wo_version": types.Int64Type,
}

var pageAccessAttrTypes = map[string]attr.Type{
	"protected":    types.BoolType,
	"session_days": types.Int64Type,
	"email":        types.ObjectType{AttrTypes: pageEmailAccessAttrTypes},
	"password":     types.ObjectType{AttrTypes: pagePasswordAccessAttrTypes},
	"rules":        types.SetType{ElemType: types.StringType},
}

func pageAccessAttribute() schema.SingleNestedAttribute {
	return schema.SingleNestedAttribute{
		Optional: true,
		Description: "Who can open the page. With `protected = true` only people a way in lets through can: the " +
			"addresses and domains `rules` names, by a code emailed to them, or anyone with the page password. Anyone " +
			"else gets a sign-in screen. Making a page private removes its webhook subscribers and the email " +
			"subscribers no way in covers, and needs a plan with private pages and a server that has them; opening " +
			"one works on any plan. Omit to manage access in the console instead: the provider then neither reads nor " +
			"changes it, and removing it leaves the page's access as it is, so set `protected = false` to open the " +
			"page to everyone again. Who signed in, and blocking them, stay in the console.",
		Validators: []validator.Object{privatePageNeedsAWayIn{}},
		Attributes: map[string]schema.Attribute{
			"protected": schema.BoolAttribute{
				Required: true,
				Description: "Only people a way in lets through can open the page. False opens it to everyone and " +
					"keeps the `rules` and password this block gives for next time; the block is still sent whole.",
			},
			"session_days": schema.Int64Attribute{
				Optional: true,
				Description: "Keep a sign-in, by email or password, for 7, 30 or 90 days. Omit to keep it until " +
					"sign-out, or until the way in it came through closes. Lived under `email` before 0.18.0.",
				Validators: []validator.Int64{int64validator.OneOf(client.PageAccessSessionDays...)},
			},
			"email": schema.SingleNestedAttribute{
				Optional: true,
				Computed: true,
				Default: objectdefault.StaticValue(types.ObjectValueMust(pageEmailAccessAttrTypes, map[string]attr.Value{
					"enabled":          types.BoolValue(emailByDefault),
					"passkeys_enabled": types.BoolValue(passkeysByDefault),
					"recheck_days":     types.Int64Null(),
				})),
				Description: "The emailed sign-in code, and the passkeys that shortcut it.",
				Attributes: map[string]schema.Attribute{
					"enabled": schema.BoolAttribute{
						Optional:    true,
						Computed:    true,
						Default:     booldefault.StaticBool(emailByDefault),
						Description: "The people `rules` names can sign in with a code emailed to them.",
					},
					"passkeys_enabled": schema.BoolAttribute{
						Optional:    true,
						Computed:    true,
						Default:     booldefault.StaticBool(passkeysByDefault),
						Description: "Let a viewer save a passkey after an email sign-in and use it next time.",
					},
					"recheck_days": schema.Int64Attribute{
						Optional: true,
						Description: "Ask a passkey viewer for an email code again once they last proved their address " +
							"longer ago than this: 30, 90 or 365 days. Omit to never ask.",
						Validators: []validator.Int64{int64validator.OneOf(client.PageAccessRecheckDays...)},
					},
				},
			},
			// Optional and never Computed: a computed object may not hold a
			// write-only value.
			"password": schema.SingleNestedAttribute{
				Optional: true,
				Description: "One password shared with everyone who should read the page. It is stored as a hash and " +
					"never read back. Omit to keep the page password off.",
				Attributes: map[string]schema.Attribute{
					"enabled": schema.BoolAttribute{
						Required: true,
						Description: "Anyone with the password can open the page. False signs out everyone who came " +
							"in with it and keeps the stored password, so turning it on again brings the same one back.",
					},
					"value_wo": schema.StringAttribute{
						Optional:  true,
						WriteOnly: true,
						Sensitive: true,
						Description: fmt.Sprintf("The password, 1 to %d characters, kept as written (Terraform 1.11+): "+
							"never in plan or state. Sent on create, when value_wo_version first appears and whenever it "+
							"changes; each send is a new password that signs out every device that used the old one, so "+
							"leave it out to keep a password set in the console. Read it from an ephemeral variable or "+
							"resource; Terraform keeps no copy to show later, so the people who need it must get it "+
							"from where it came from.", client.MaxPagePasswordChars),
						Validators: []validator.String{
							stringvalidator.UTF8LengthBetween(1, client.MaxPagePasswordChars),
							stringvalidator.AlsoRequires(path.MatchRelative().AtParent().AtName("value_wo_version")),
						},
					},
					"value_wo_version": schema.Int64Attribute{
						Optional:    true,
						Description: "Change trigger for value_wo. The password itself never diffs, so bump this to set a new one.",
						Validators: []validator.Int64{
							int64validator.AlsoRequires(path.MatchRelative().AtParent().AtName("value_wo")),
						},
					},
				},
			},
			"rules": schema.SetAttribute{
				Optional:    true,
				Computed:    true,
				Default:     setdefault.StaticValue(types.SetValueMust(types.StringType, []attr.Value{})),
				ElementType: types.StringType,
				Description: fmt.Sprintf("Who can get an email code, up to %d: an address such as `anna@example.com`, "+
					"or a domain such as `example.com` for every address there. Written in lower case, an "+
					"internationalised domain in its `xn--` form. Omitted or `[]` empties the list.", client.MaxPageAccessRules),
				Validators: []validator.Set{
					setvalidator.SizeAtMost(client.MaxPageAccessRules),
					setvalidator.ValueStringsAre(pageAccessRuleValidator{}),
				},
			},
		},
	}
}

// privatePageNeedsAWayIn refuses at plan what the API refuses at apply: a
// private page nobody can open, or an email code nobody can get.
type privatePageNeedsAWayIn struct{}

func (privatePageNeedsAWayIn) Description(context.Context) string {
	return "a private page needs a way in: the email code with at least one address or domain in rules, or the password"
}

func (v privatePageNeedsAWayIn) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

func (v privatePageNeedsAWayIn) ValidateObject(ctx context.Context, req validator.ObjectRequest, resp *validator.ObjectResponse) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}
	attrs := req.ConfigValue.Attributes()
	protected, _ := attrs["protected"].(types.Bool)
	if !protected.ValueBool() {
		return
	}
	email, known := boolUnder(attrs["email"], "enabled", emailByDefault)
	if !known {
		return
	}
	if email {
		rules, _ := attrs["rules"].(types.Set)
		if !rules.IsUnknown() && len(rules.Elements()) == 0 {
			msg := "with the email code on, a private page needs at least one address or domain in rules"
			if on, known := boolUnder(attrs["password"], "enabled", false); known && on {
				msg += "; for a page opened by the password alone, set email = { enabled = false }"
			}
			resp.Diagnostics.AddAttributeError(req.Path.AtName("rules"), "Missing access rules", msg)
		}
		return
	}
	password, known := boolUnder(attrs["password"], "enabled", false)
	if known && !password {
		resp.Diagnostics.AddAttributeError(req.Path, "No way in", v.Description(ctx))
	}
}

// boolUnder reads a bool in a nested object, `fallback` for one the config
// leaves out; known is false while either is not yet known.
func boolUnder(v attr.Value, name string, fallback bool) (value, known bool) {
	obj, _ := v.(types.Object)
	if obj.IsUnknown() {
		return false, false
	}
	if obj.IsNull() {
		return fallback, true
	}
	b, _ := obj.Attributes()[name].(types.Bool)
	if b.IsUnknown() {
		return false, false
	}
	if b.IsNull() {
		return fallback, true
	}
	return b.ValueBool(), true
}

// pageAccessRuleValidator takes an entry only in the spelling the API stores,
// so the list read back matches the one configured.
type pageAccessRuleValidator struct{}

func (pageAccessRuleValidator) Description(context.Context) string {
	return "must be an address such as anna@example.com or a domain such as example.com, in lower case, " +
		"an internationalised domain in its xn-- form"
}

func (v pageAccessRuleValidator) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

func (v pageAccessRuleValidator) ValidateString(ctx context.Context, req validator.StringRequest, resp *validator.StringResponse) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}
	raw := req.ConfigValue.ValueString()
	if accessRuleIsCanonical(raw) {
		return
	}
	msg := v.Description(ctx)
	fixed := strings.ToLower(strings.TrimSuffix(strings.TrimPrefix(strings.TrimSpace(raw), "@"), "."))
	if accessRuleIsCanonical(fixed) {
		msg += fmt.Sprintf(": write %q", fixed)
	}
	resp.Diagnostics.AddAttributeError(req.Path, "Invalid Attribute Value", msg)
}

func accessRuleIsCanonical(v string) bool {
	local, domain, isAddress := strings.Cut(v, "@")
	if !isAddress {
		return hostIsCanonical(v)
	}
	return local != "" && len(v) <= client.MaxPageAccessAddressLen && local == strings.ToLower(local) &&
		!strings.ContainsFunc(local, func(r rune) bool { return unicode.IsSpace(r) || unicode.IsControl(r) }) &&
		hostIsCanonical(domain)
}

// hostIsCanonical mirrors the API's host normaliser on input it leaves as is:
// two or more legal DNS labels, lower case ASCII, no trailing dot.
func hostIsCanonical(host string) bool {
	if len(host) > 253 {
		return false
	}
	labels := strings.Split(host, ".")
	if len(labels) < 2 {
		return false
	}
	for _, l := range labels {
		if len(l) == 0 || len(l) > 63 || l[0] == '-' || l[len(l)-1] == '-' {
			return false
		}
		for i := 0; i < len(l); i++ {
			c := l[i]
			if (c < 'a' || c > 'z') && (c < '0' || c > '9') && c != '-' {
				return false
			}
		}
	}
	return true
}

// pageAccessToWire is nil when the plan does not decide access, which leaves
// the page's access as it is. Once decided, every attribute under it is known:
// email, its switches and rules carry defaults, and a password the config
// leaves out is off. A value is sent only once grafted in for this apply.
func pageAccessToWire(obj types.Object) *client.PageAccess {
	if obj.IsNull() || obj.IsUnknown() {
		return nil
	}
	a := obj.Attributes()
	e := a["email"].(types.Object).Attributes()
	out := &client.PageAccess{
		Protected:   a["protected"].(types.Bool).ValueBool(),
		SessionDays: a["session_days"].(types.Int64).ValueInt64Pointer(),
		Email: client.PageEmailAccess{
			Enabled:         e["enabled"].(types.Bool).ValueBool(),
			PasskeysEnabled: e["passkeys_enabled"].(types.Bool).ValueBool(),
			RecheckDays:     e["recheck_days"].(types.Int64).ValueInt64Pointer(),
		},
		Rules: []client.PageAccessRule{},
	}
	if pw, ok := passwordBlock(obj); ok {
		out.Password.Enabled = pw["enabled"].(types.Bool).ValueBool()
		out.Password.Value = pw["value_wo"].(types.String).ValueStringPointer()
	}
	for _, el := range a["rules"].(types.Set).Elements() {
		value := el.(types.String).ValueString()
		kind := client.AccessRuleDomain
		if strings.Contains(value, "@") {
			kind = client.AccessRuleAddress
		}
		out.Rules = append(out.Rules, client.PageAccessRule{Kind: kind, Value: value})
	}
	return out
}

// passwordBlock is the password object under a known access, when the
// configuration has one.
func passwordBlock(access types.Object) (map[string]attr.Value, bool) {
	if access.IsNull() || access.IsUnknown() {
		return nil, false
	}
	pw, _ := access.Attributes()["password"].(types.Object)
	if pw.IsNull() || pw.IsUnknown() {
		return nil, false
	}
	return pw.Attributes(), true
}

// graftPagePassword copies the write-only password from the decoded config
// into the planned access, where Terraform never carries it, when this apply
// is to send it: sending one signs out everyone who used the old, so only a
// create or a changed version does.
func graftPagePassword(plan *statusPageModel, cfg statusPageModel, send bool) {
	pw, ok := passwordBlock(plan.Access)
	if !ok {
		return
	}
	from, ok := passwordBlock(cfg.Access)
	if !send || !ok {
		return
	}
	grafted := maps.Clone(pw)
	grafted["value_wo"] = from["value_wo"]
	access := maps.Clone(plan.Access.Attributes())
	access["password"] = types.ObjectValueMust(pagePasswordAccessAttrTypes, grafted)
	plan.Access = types.ObjectValueMust(pageAccessAttrTypes, access)
}

// pagePasswordRotated says an update must send the password: its version
// moved since the last apply.
func pagePasswordRotated(plan, state types.Object) bool {
	return !pagePasswordVersion(plan).Equal(pagePasswordVersion(state))
}

func pagePasswordVersion(access types.Object) types.Int64 {
	pw, ok := passwordBlock(access)
	if !ok {
		return types.Int64Null()
	}
	v, _ := pw["value_wo_version"].(types.Int64)
	return v
}

// newPagePasswordNeedsValue refuses turning the password on for a page that
// does not exist yet without one to store.
func newPagePasswordNeedsValue(access types.Object) diag.Diagnostics {
	var diags diag.Diagnostics
	pw, ok := passwordBlock(access)
	if !ok {
		return diags
	}
	enabled, _ := pw["enabled"].(types.Bool)
	value, _ := pw["value_wo"].(types.String)
	if enabled.ValueBool() && value.IsNull() {
		diags.AddAttributeError(path.Root("access").AtName("password").AtName("value_wo"), "Missing page password",
			"a new page has no password for enabled = true to turn on: set value_wo and value_wo_version")
	}
	return diags
}

// pageAccessToModel reads the page's access only where the configuration
// manages it: a null prior stays null, whatever the console set.
func pageAccessToModel(prior types.Object, a *client.PageAccess) types.Object {
	if prior.IsNull() || a == nil {
		return types.ObjectNull(pageAccessAttrTypes)
	}
	rules := make([]attr.Value, 0, len(a.Rules))
	for _, r := range a.Rules {
		rules = append(rules, types.StringValue(r.Value))
	}
	return types.ObjectValueMust(pageAccessAttrTypes, map[string]attr.Value{
		"protected":    types.BoolValue(a.Protected),
		"session_days": types.Int64PointerValue(a.SessionDays),
		"email": types.ObjectValueMust(pageEmailAccessAttrTypes, map[string]attr.Value{
			"enabled":          types.BoolValue(a.Email.Enabled),
			"passkeys_enabled": types.BoolValue(a.Email.PasskeysEnabled),
			"recheck_days":     types.Int64PointerValue(a.Email.RecheckDays),
		}),
		"password": pagePasswordToModel(prior, a.Password),
		"rules":    types.SetValueMust(types.StringType, rules),
	})
}

// A config without a password block reads back as one only when the page has
// the password on, so that drift shows; the value is never read, and the
// version is the config's own.
func pagePasswordToModel(prior types.Object, p client.PagePasswordAccess) types.Object {
	if _, ok := passwordBlock(prior); !ok && !p.Enabled {
		return types.ObjectNull(pagePasswordAccessAttrTypes)
	}
	return types.ObjectValueMust(pagePasswordAccessAttrTypes, map[string]attr.Value{
		"enabled":          types.BoolValue(p.Enabled),
		"value_wo":         types.StringNull(),
		"value_wo_version": pagePasswordVersion(prior),
	})
}

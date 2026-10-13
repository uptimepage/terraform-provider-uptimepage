package provider

import (
	"context"
	"fmt"
	"strings"
	"unicode"

	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/setvalidator"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/objectdefault"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/uptimepage/terraform-provider-uptimepage/internal/client"
)

const passkeysByDefault = true

var pageEmailAccessAttrTypes = map[string]attr.Type{
	"passkeys_enabled": types.BoolType,
	"recheck_days":     types.Int64Type,
	"session_days":     types.Int64Type,
}

var pageAccessAttrTypes = map[string]attr.Type{
	"protected": types.BoolType,
	"email":     types.ObjectType{AttrTypes: pageEmailAccessAttrTypes},
	"rules":     types.SetType{ElemType: types.StringType},
}

func pageAccessAttribute() schema.SingleNestedAttribute {
	return schema.SingleNestedAttribute{
		Optional: true,
		Description: "Who can open the page. With `protected = true` only the people `rules` names can: anyone else gets " +
			"a sign-in screen, and a listed person signs in with a code emailed to them. Making a page private removes its " +
			"webhook subscribers and the email subscribers the list does not cover, and needs a plan with private pages " +
			"and a server that has them; opening one works on any plan. Omit to manage access in the console instead: " +
			"the provider then neither reads nor changes it, and removing it leaves the page's access as it is, so set " +
			"`protected = false` to open the page to everyone again. Who signed in, and blocking them, stay in the console.",
		Validators: []validator.Object{privatePageNeedsRules{}},
		Attributes: map[string]schema.Attribute{
			"protected": schema.BoolAttribute{
				Required: true,
				Description: "Only the people `rules` names can open the page. False opens it to everyone, and the " +
					"list stays stored as `rules` gives it, for next time.",
			},
			"email": schema.SingleNestedAttribute{
				Optional: true,
				Computed: true,
				Default: objectdefault.StaticValue(types.ObjectValueMust(pageEmailAccessAttrTypes, map[string]attr.Value{
					"passkeys_enabled": types.BoolValue(passkeysByDefault),
					"recheck_days":     types.Int64Null(),
					"session_days":     types.Int64Null(),
				})),
				Description: "The emailed sign-in code, and the passkeys that shortcut it.",
				Attributes: map[string]schema.Attribute{
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
					"session_days": schema.Int64Attribute{
						Optional: true,
						Description: "Keep a sign-in for 7, 30 or 90 days. Omit to keep it until the viewer signs out, " +
							"is blocked, or the list stops covering them.",
						Validators: []validator.Int64{int64validator.OneOf(client.PageAccessSessionDays...)},
					},
				},
			},
			// Required, so opening a page by writing protected = false cannot also
			// empty the list it keeps for next time.
			"rules": schema.SetAttribute{
				Required:    true,
				ElementType: types.StringType,
				Description: fmt.Sprintf("Who can open it, up to %d: an address such as `anna@example.com`, or a domain "+
					"such as `example.com` for every address there. Written in lower case, an internationalised domain "+
					"in its `xn--` form. `[]` empties the list.", client.MaxPageAccessRules),
				Validators: []validator.Set{
					setvalidator.SizeAtMost(client.MaxPageAccessRules),
					setvalidator.ValueStringsAre(pageAccessRuleValidator{}),
				},
			},
		},
	}
}

// privatePageNeedsRules refuses at plan what the API refuses at apply: a
// private page nobody can open.
type privatePageNeedsRules struct{}

func (privatePageNeedsRules) Description(context.Context) string {
	return "a private page needs at least one address or domain in rules"
}

func (v privatePageNeedsRules) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

func (v privatePageNeedsRules) ValidateObject(ctx context.Context, req validator.ObjectRequest, resp *validator.ObjectResponse) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}
	attrs := req.ConfigValue.Attributes()
	protected, _ := attrs["protected"].(types.Bool)
	rules, _ := attrs["rules"].(types.Set)
	if !protected.ValueBool() || rules.IsUnknown() || len(rules.Elements()) > 0 {
		return
	}
	resp.Diagnostics.AddAttributeError(req.Path.AtName("rules"), "Missing access rules", v.Description(ctx))
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
// email and passkeys_enabled carry defaults.
func pageAccessToWire(obj types.Object) *client.PageAccess {
	if obj.IsNull() || obj.IsUnknown() {
		return nil
	}
	a := obj.Attributes()
	e := a["email"].(types.Object).Attributes()
	out := &client.PageAccess{
		Protected: a["protected"].(types.Bool).ValueBool(),
		Email: client.PageEmailAccess{
			PasskeysEnabled: e["passkeys_enabled"].(types.Bool).ValueBool(),
			RecheckDays:     e["recheck_days"].(types.Int64).ValueInt64Pointer(),
			SessionDays:     e["session_days"].(types.Int64).ValueInt64Pointer(),
		},
		Rules: []client.PageAccessRule{},
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
		"protected": types.BoolValue(a.Protected),
		"email": types.ObjectValueMust(pageEmailAccessAttrTypes, map[string]attr.Value{
			"passkeys_enabled": types.BoolValue(a.Email.PasskeysEnabled),
			"recheck_days":     types.Int64PointerValue(a.Email.RecheckDays),
			"session_days":     types.Int64PointerValue(a.Email.SessionDays),
		}),
		"rules": types.SetValueMust(types.StringType, rules),
	})
}

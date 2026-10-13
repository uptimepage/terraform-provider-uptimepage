package provider

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/uptimepage/terraform-provider-uptimepage/internal/client"
)

func accessObject(protected bool, passkeys bool, recheck *int64, rules ...string) types.Object {
	values := make([]attr.Value, 0, len(rules))
	for _, r := range rules {
		values = append(values, types.StringValue(r))
	}
	return types.ObjectValueMust(pageAccessAttrTypes, map[string]attr.Value{
		"protected": types.BoolValue(protected),
		"email": types.ObjectValueMust(pageEmailAccessAttrTypes, map[string]attr.Value{
			"passkeys_enabled": types.BoolValue(passkeys),
			"recheck_days":     types.Int64PointerValue(recheck),
			"session_days":     types.Int64Null(),
		}),
		"rules": types.SetValueMust(types.StringType, values),
	})
}

func TestStatusPageToUpdate_SendsAccessWhole(t *testing.T) {
	ninety := int64(90)
	body, err := json.Marshal(statusPageModel{
		Slug:   types.StringValue("weber"),
		Name:   types.StringValue("Autohaus Weber"),
		Access: accessObject(true, false, &ninety, "dealer.example", "k.lindner@example.org"),
	}.toUpdate())
	if err != nil {
		t.Fatal(err)
	}
	var wire struct {
		Access struct {
			Protected bool                       `json:"protected"`
			Email     map[string]json.RawMessage `json:"email"`
			Rules     []client.PageAccessRule    `json:"rules"`
		} `json:"access"`
	}
	if err := json.Unmarshal(body, &wire); err != nil {
		t.Fatal(err)
	}
	if !wire.Access.Protected {
		t.Errorf("protected = false: %s", body)
	}
	for key, want := range map[string]string{"passkeys_enabled": "false", "recheck_days": "90", "session_days": "null"} {
		if got := string(wire.Access.Email[key]); got != want {
			t.Errorf("email.%s = %s, want %s", key, got, want)
		}
	}
	// A private page cannot turn its only way in off, so the switch is the
	// server's to default.
	if _, sent := wire.Access.Email["enabled"]; sent {
		t.Errorf("email.enabled must not be sent: %s", body)
	}
	kinds := map[string]string{}
	for _, r := range wire.Access.Rules {
		kinds[r.Value] = r.Kind
	}
	if kinds["dealer.example"] != "domain" || kinds["k.lindner@example.org"] != "address" || len(kinds) != 2 {
		t.Errorf("rules = %+v", wire.Access.Rules)
	}
}

// An access the config leaves out stays off the wire, which the API reads as
// "keep the page's access".
func TestStatusPageToUpdate_OmitsAnUndecidedAccess(t *testing.T) {
	for _, access := range []types.Object{types.ObjectNull(pageAccessAttrTypes), types.ObjectUnknown(pageAccessAttrTypes)} {
		body, err := json.Marshal(statusPageModel{
			Slug:   types.StringValue("weber"),
			Name:   types.StringValue("Autohaus Weber"),
			Access: access,
		}.toUpdate())
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(body), `"access"`) {
			t.Errorf("%v access must not be sent: %s", access, body)
		}
	}
}

// Access rides the create, so a plan without private pages refuses it before
// any page exists.
func TestStatusPageToNew_CarriesAccess(t *testing.T) {
	m := statusPageModel{Slug: types.StringValue("weber"), Name: types.StringValue("Autohaus Weber")}
	m.Access = types.ObjectNull(pageAccessAttrTypes)
	if m.toNew().Access != nil {
		t.Error("unmanaged access sent on create")
	}
	m.Access = accessObject(true, true, nil, "dealer.example")
	if a := m.toNew().Access; a == nil || !a.Protected || len(a.Rules) != 1 {
		t.Errorf("create access = %+v", a)
	}
}

func TestStatusPageToModel_ReadsAccessTheConfigManages(t *testing.T) {
	page := &client.StatusPage{
		ID: "id", Slug: "weber", Name: "Autohaus Weber", PublicStyle: "default",
		Access: &client.PageAccess{
			Protected: true,
			Email:     client.PageEmailAccess{PasskeysEnabled: true},
			Rules: []client.PageAccessRule{
				{Kind: client.AccessRuleAddress, Value: "k.lindner@example.org"},
				{Kind: client.AccessRuleDomain, Value: "partner.example"},
			},
		},
	}
	managed := statusPageModel{Access: accessObject(true, true, nil, "dealer.example")}
	want := accessObject(true, true, nil, "partner.example", "k.lindner@example.org")
	if got := statusPageToModel(managed, page).Access; !got.Equal(want) {
		t.Errorf("access = %v, want the page's %v", got, want)
	}
	// Left to the console: nothing to diff, whatever the page has.
	if got := statusPageToModel(statusPageModel{Access: types.ObjectNull(pageAccessAttrTypes)}, page).Access; !got.IsNull() {
		t.Errorf("unmanaged access = %v, want null", got)
	}
}

func TestPageAccess_RoundTripsThroughTheWire(t *testing.T) {
	thirty := int64(30)
	in := accessObject(false, true, &thirty, "dealer.example", "anna@dealer.example", "xn--mller-kva.de")
	if got := pageAccessToModel(in, pageAccessToWire(in)); !got.Equal(in) {
		t.Errorf("round trip = %v, want %v", got, in)
	}
}

func TestPageAccessRule_TakesOnlyTheStoredSpelling(t *testing.T) {
	for value, ok := range map[string]bool{
		"dealer.example":             true,
		"a-b.co.uk":                  true,
		"xn--mller-kva.de":           true,
		"k.lindner@example.org":      true,
		"anna+status@dealer.example": true,
		"DEALER.example":             false,
		"@dealer.example":            false,
		"dealer.example.":            false,
		"dealer":                     false,
		"-dealer.example":            false,
		"dealer-.example":            false,
		"müller.de":                  false,
		"dealer.example:443":         false,
		"Anna@dealer.example":        false,
		"anna@DEALER.example":        false,
		"anna@dealer":                false,
		"@":                          false,
		"anna@":                      false,
		"an na@dealer.example":       false,
		"anna@team@dealer.example":   false,
		" dealer.example":            false,
	} {
		if got := accessRuleIsCanonical(value); got != ok {
			t.Errorf("%q canonical = %v, want %v", value, got, ok)
		}
	}
}

func TestPageAccessRule_NamesTheSpellingToWrite(t *testing.T) {
	for value, hint := range map[string]string{
		"DEALER.example":      `write "dealer.example"`,
		"dealer.example.":     `write "dealer.example"`,
		"@dealer.example":     `write "dealer.example"`,
		"Anna@Dealer.EXAMPLE": `write "anna@dealer.example"`,
		"müller.de":           "",
	} {
		var resp validator.StringResponse
		pageAccessRuleValidator{}.ValidateString(context.Background(), validator.StringRequest{
			Path:        path.Root("access").AtName("rules"),
			ConfigValue: types.StringValue(value),
		}, &resp)
		if !resp.Diagnostics.HasError() {
			t.Errorf("%q passed", value)
			continue
		}
		detail := resp.Diagnostics[0].Detail()
		if hint == "" && strings.Contains(detail, "write") || hint != "" && !strings.Contains(detail, hint) {
			t.Errorf("%q detail = %q, want hint %q", value, detail, hint)
		}
	}
}

func TestPrivatePage_NeedsSomeoneOnTheList(t *testing.T) {
	for name, tc := range map[string]struct {
		access types.Object
		refuse bool
	}{
		"private, empty":  {accessObject(true, true, nil), true},
		"private, listed": {accessObject(true, true, nil, "dealer.example"), false},
		"public, empty":   {accessObject(false, true, nil), false},
		"left out":        {types.ObjectNull(pageAccessAttrTypes), false},
		"rules not yet known": {types.ObjectValueMust(pageAccessAttrTypes, map[string]attr.Value{
			"protected": types.BoolValue(true),
			"email":     types.ObjectNull(pageEmailAccessAttrTypes),
			"rules":     types.SetUnknown(types.StringType),
		}), false},
	} {
		var resp validator.ObjectResponse
		privatePageNeedsRules{}.ValidateObject(context.Background(), validator.ObjectRequest{
			Path:        path.Root("access"),
			ConfigValue: tc.access,
		}, &resp)
		if resp.Diagnostics.HasError() != tc.refuse {
			t.Errorf("%s: refused = %v, want %v", name, resp.Diagnostics.HasError(), tc.refuse)
		}
	}
}

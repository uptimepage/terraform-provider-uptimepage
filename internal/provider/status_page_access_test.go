package provider

import (
	"context"
	"encoding/json"
	"maps"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/uptimepage/terraform-provider-uptimepage/internal/client"
)

func accessObject(protected bool, passkeys bool, recheck *int64, rules ...string) types.Object {
	return accessWith(protected, true, passkeys, recheck, types.ObjectNull(pagePasswordAccessAttrTypes), rules...)
}

func passwordObject(enabled bool, version *int64) types.Object {
	return types.ObjectValueMust(pagePasswordAccessAttrTypes, map[string]attr.Value{
		"enabled":          types.BoolValue(enabled),
		"value_wo":         types.StringNull(),
		"value_wo_version": types.Int64PointerValue(version),
	})
}

func accessWith(protected, email, passkeys bool, recheck *int64, password types.Object, rules ...string) types.Object {
	values := make([]attr.Value, 0, len(rules))
	for _, r := range rules {
		values = append(values, types.StringValue(r))
	}
	return types.ObjectValueMust(pageAccessAttrTypes, map[string]attr.Value{
		"protected":    types.BoolValue(protected),
		"session_days": types.Int64Null(),
		"email": types.ObjectValueMust(pageEmailAccessAttrTypes, map[string]attr.Value{
			"enabled":          types.BoolValue(email),
			"passkeys_enabled": types.BoolValue(passkeys),
			"recheck_days":     types.Int64PointerValue(recheck),
		}),
		"password": password,
		"rules":    types.SetValueMust(types.StringType, values),
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
			Protected   bool                       `json:"protected"`
			SessionDays json.RawMessage            `json:"session_days"`
			Email       map[string]json.RawMessage `json:"email"`
			Password    map[string]json.RawMessage `json:"password"`
			Rules       []client.PageAccessRule    `json:"rules"`
		} `json:"access"`
	}
	if err := json.Unmarshal(body, &wire); err != nil {
		t.Fatal(err)
	}
	if !wire.Access.Protected || string(wire.Access.SessionDays) != "null" {
		t.Errorf("protected and session_days: %s", body)
	}
	for key, want := range map[string]string{"enabled": "true", "passkeys_enabled": "false", "recheck_days": "90"} {
		if got := string(wire.Access.Email[key]); got != want {
			t.Errorf("email.%s = %s, want %s", key, got, want)
		}
	}
	// No password block is the password off, and no value keeps the stored one.
	if got := string(wire.Access.Password["enabled"]); got != "false" || len(wire.Access.Password) != 1 {
		t.Errorf("password = %s, want only enabled false", body)
	}
	kinds := map[string]string{}
	for _, r := range wire.Access.Rules {
		kinds[r.Value] = r.Kind
	}
	if kinds["dealer.example"] != "domain" || kinds["k.lindner@example.org"] != "address" || len(kinds) != 2 {
		t.Errorf("rules = %+v", wire.Access.Rules)
	}
}

func withValue(access types.Object, value string) types.Object {
	pw := maps.Clone(access.Attributes()["password"].(types.Object).Attributes())
	pw["value_wo"] = types.StringValue(value)
	a := maps.Clone(access.Attributes())
	a["password"] = types.ObjectValueMust(pagePasswordAccessAttrTypes, pw)
	return types.ObjectValueMust(pageAccessAttrTypes, a)
}

// The password travels only when grafted in from config for this apply, as
// set; the plan never holds it.
func TestGraftPagePassword_SendsTheValueOnlyWhenAsked(t *testing.T) {
	one := int64(1)
	plan := statusPageModel{
		Slug:   types.StringValue("weber"),
		Name:   types.StringValue("Autohaus Weber"),
		Access: accessWith(true, false, true, nil, passwordObject(true, &one)),
	}
	cfg := plan
	cfg.Access = withValue(plan.Access, "open me ")
	for send, want := range map[bool]*string{false: nil, true: ptrTo("open me ")} {
		m := plan
		graftPagePassword(&m, cfg, send)
		got := m.toUpdate().Access.Password
		if !got.Enabled || (got.Value == nil) != (want == nil) || got.Value != nil && *got.Value != *want {
			t.Errorf("send=%v: password = %+v", send, got)
		}
	}
	body, _ := json.Marshal(plan.toUpdate())
	if strings.Contains(string(body), `"value"`) {
		t.Errorf("a kept password must not send a value: %s", body)
	}
	// Grafted for the request, never into state.
	m := plan
	graftPagePassword(&m, cfg, true)
	page := &client.StatusPage{ID: "id", Slug: "weber", Name: "Autohaus Weber", PublicStyle: "default",
		Access: &client.PageAccess{Protected: true, Password: client.PagePasswordAccess{Enabled: true}}}
	if got := statusPageToModel(m, page).Access.Attributes()["password"]; !got.Equal(passwordObject(true, &one)) {
		t.Errorf("state password = %v", got)
	}
}

func ptrTo[T any](v T) *T { return &v }

func TestNewPage_NeedsAPasswordToTurnOn(t *testing.T) {
	one := int64(1)
	on := accessWith(true, false, true, nil, passwordObject(true, nil))
	if d := newPagePasswordNeedsValue(on); !d.HasError() {
		t.Error("enabled with no value on a new page passed")
	}
	for name, access := range map[string]types.Object{
		"with a value": withValue(accessWith(true, false, true, nil, passwordObject(true, &one)), "x"),
		"off":          accessWith(true, true, true, nil, passwordObject(false, nil), "dealer.example"),
		"no block":     accessObject(true, true, nil, "dealer.example"),
		"no access":    types.ObjectNull(pageAccessAttrTypes),
	} {
		if d := newPagePasswordNeedsValue(access); d.HasError() {
			t.Errorf("%s: refused: %v", name, d)
		}
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
// any page exists, and the password with it.
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
	one := int64(1)
	m.Access = accessWith(true, false, true, nil, passwordObject(true, &one))
	cfg := m
	cfg.Access = withValue(m.Access, "shared")
	graftPagePassword(&m, cfg, true)
	if a := m.toNew().Access; a == nil || !a.Password.Enabled || a.Password.Value == nil {
		t.Errorf("create password = %+v", a)
	}
}

func TestStatusPageToModel_ReadsAccessTheConfigManages(t *testing.T) {
	page := &client.StatusPage{
		ID: "id", Slug: "weber", Name: "Autohaus Weber", PublicStyle: "default",
		Access: &client.PageAccess{
			Protected: true,
			Email:     client.PageEmailAccess{Enabled: true, PasskeysEnabled: true},
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

func TestStatusPageToModel_ReadsThePasswordButNeverItsValue(t *testing.T) {
	two := int64(2)
	page := &client.StatusPage{
		ID: "id", Slug: "weber", Name: "Autohaus Weber", PublicStyle: "default",
		Access: &client.PageAccess{
			Protected: true,
			Password:  client.PagePasswordAccess{Enabled: true},
		},
	}
	// The version is the config's own; the value never comes back.
	configured := statusPageModel{Access: accessWith(true, false, true, nil, passwordObject(true, &two))}
	got := statusPageToModel(configured, page).Access.Attributes()["password"]
	if !got.Equal(passwordObject(true, &two)) {
		t.Errorf("password = %v", got)
	}
	// Turned on in the console under a config without one: drift to undo.
	bare := statusPageModel{Access: accessWith(true, false, true, nil, types.ObjectNull(pagePasswordAccessAttrTypes))}
	if got := statusPageToModel(bare, page).Access.Attributes()["password"]; !got.Equal(passwordObject(true, nil)) {
		t.Errorf("console password = %v, want it shown as on", got)
	}
	page.Access.Password.Enabled = false
	if got := statusPageToModel(bare, page).Access.Attributes()["password"]; !got.(types.Object).IsNull() {
		t.Errorf("an off password under no block = %v, want null", got)
	}
}

func TestPagePassword_IsSentWhenItsVersionMoves(t *testing.T) {
	one, two := int64(1), int64(2)
	none := types.ObjectNull(pagePasswordAccessAttrTypes)
	for name, tc := range map[string]struct {
		plan, state types.Object
		rotated     bool
	}{
		"same version":     {passwordObject(true, &one), passwordObject(true, &one), false},
		"bumped":           {passwordObject(true, &two), passwordObject(true, &one), true},
		"first set":        {passwordObject(true, &one), none, true},
		"none either side": {none, none, false},
		"turned off, kept": {passwordObject(false, &one), passwordObject(true, &one), false},
		"version dropped":  {passwordObject(true, nil), passwordObject(true, &one), true},
	} {
		plan := accessWith(true, false, true, nil, tc.plan)
		state := accessWith(true, false, true, nil, tc.state)
		if got := pagePasswordRotated(plan, state); got != tc.rotated {
			t.Errorf("%s: rotated = %v, want %v", name, got, tc.rotated)
		}
	}
	if !pagePasswordRotated(accessWith(true, false, true, nil, passwordObject(true, &one)), types.ObjectNull(pageAccessAttrTypes)) {
		t.Error("access newly managed with a password: rotated = false")
	}
}

func TestPageAccess_RoundTripsThroughTheWire(t *testing.T) {
	thirty := int64(30)
	in := accessObject(false, true, &thirty, "dealer.example", "anna@dealer.example", "xn--mller-kva.de")
	if got := pageAccessToModel(in, pageAccessToWire(in)); !got.Equal(in) {
		t.Errorf("round trip = %v, want %v", got, in)
	}
	one := int64(1)
	withPassword := accessWith(true, false, false, nil, passwordObject(true, &one))
	if got := pageAccessToModel(withPassword, pageAccessToWire(withPassword)); !got.Equal(withPassword) {
		t.Errorf("round trip = %v, want %v", got, withPassword)
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

func TestPrivatePage_NeedsAWayIn(t *testing.T) {
	on := passwordObject(true, nil)
	off := passwordObject(false, nil)
	none := types.ObjectNull(pagePasswordAccessAttrTypes)
	for name, tc := range map[string]struct {
		access types.Object
		refuse bool
	}{
		"private, empty":            {accessObject(true, true, nil), true},
		"private, listed":           {accessObject(true, true, nil, "dealer.example"), false},
		"public, empty":             {accessObject(false, true, nil), false},
		"email off, password on":    {accessWith(true, false, true, nil, on), false},
		"email off, password off":   {accessWith(true, false, true, nil, off), true},
		"email off, no password":    {accessWith(true, false, true, nil, none), true},
		"email on, password on":     {accessWith(true, true, true, nil, on), true},
		"public, nothing on at all": {accessWith(false, false, true, nil, none), false},
		"left out":                  {types.ObjectNull(pageAccessAttrTypes), false},
		"rules not yet known": {types.ObjectValueMust(pageAccessAttrTypes, map[string]attr.Value{
			"protected":    types.BoolValue(true),
			"session_days": types.Int64Null(),
			"email":        types.ObjectNull(pageEmailAccessAttrTypes),
			"password":     none,
			"rules":        types.SetUnknown(types.StringType),
		}), false},
		"password not yet known": {accessWith(true, false, true, nil, types.ObjectUnknown(pagePasswordAccessAttrTypes)), false},
	} {
		var resp validator.ObjectResponse
		privatePageNeedsAWayIn{}.ValidateObject(context.Background(), validator.ObjectRequest{
			Path:        path.Root("access"),
			ConfigValue: tc.access,
		}, &resp)
		if resp.Diagnostics.HasError() != tc.refuse {
			t.Errorf("%s: refused = %v, want %v", name, resp.Diagnostics.HasError(), tc.refuse)
		}
	}
	// A password-only page that forgot to turn the email code off is told so.
	var resp validator.ObjectResponse
	privatePageNeedsAWayIn{}.ValidateObject(context.Background(), validator.ObjectRequest{
		Path:        path.Root("access"),
		ConfigValue: accessWith(true, true, true, nil, on),
	}, &resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("password on, email on, no rules: passed")
	}
	if !strings.Contains(resp.Diagnostics[0].Detail(), "email = { enabled = false }") {
		t.Errorf("detail = %q", resp.Diagnostics[0].Detail())
	}
}

package client

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

// TestExpectedStatus_RoundTrip pins the adjacently-tagged encoding for all three
// kinds. The range case is the load-bearing one: payload nests under "value".
func TestExpectedStatus_RoundTrip(t *testing.T) {
	cases := []struct {
		name string
		val  ExpectedStatus
		wire string
	}{
		{"exact", ExpectedStatus{Kind: StatusKindExact, Exact: 200}, `{"kind":"exact","value":200}`},
		{"range", ExpectedStatus{Kind: StatusKindRange, Range: &StatusRange{Min: 200, Max: 299}}, `{"kind":"range","value":{"min":200,"max":299}}`},
		{"one_of", ExpectedStatus{Kind: StatusKindOneOf, OneOf: []uint16{200, 204}}, `{"kind":"one_of","value":[200,204]}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := json.Marshal(tc.val)
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}
			if string(got) != tc.wire {
				t.Errorf("marshal = %s, want %s", got, tc.wire)
			}
			var back ExpectedStatus
			if err := json.Unmarshal([]byte(tc.wire), &back); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			if !reflect.DeepEqual(back, tc.val) {
				t.Errorf("round-trip = %+v, want %+v", back, tc.val)
			}
		})
	}
}

// TestNewTarget_EmptyCollectionsNeverNull guards the wire-breaker: nil tags /
// alerts must be omitted (server's serde default fires on absence, rejects
// null), and nil headers must marshal as {} (the field is mandatory, rejects
// both null and absence).
func TestNewTarget_EmptyCollectionsNeverNull(t *testing.T) {
	nt := NewTarget{
		Name:     "t",
		Interval: 60,
		Enabled:  true,
		Check: CheckSpec{Type: CheckTypeHTTP, HTTP: &HTTPCheck{
			URL:            "https://example.com",
			Method:         "GET",
			Timeout:        5000,
			ExpectedStatus: ExpectedStatus{Kind: StatusKindExact, Exact: 200},
			// Headers intentionally nil.
		}},
		// Tags and Alerts intentionally nil.
	}
	raw, err := json.Marshal(nt)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	s := string(raw)
	if strings.Contains(s, `"tags":null`) || strings.Contains(s, `"alerts":null`) {
		t.Errorf("nil tags/alerts must be omitted, got: %s", s)
	}
	if !strings.Contains(s, `"headers":{}`) {
		t.Errorf("nil headers must marshal as {}, got: %s", s)
	}
}

func TestCheckSpec_VariantsRoundTrip(t *testing.T) {
	resolver := "1.1.1.1"
	fillSel := "#username"
	cases := map[string]CheckSpec{
		"tcp":           {Type: CheckTypeTCP, TCP: &TCPCheck{Host: "db", Port: 5432, Timeout: 3000}},
		"tls_cert":      {Type: CheckTypeTLSCert, TLSCert: &TLSCertCheck{Host: "x", Port: 443, WarnDays: 30, CriticalDays: 7, Timeout: 5000}},
		"domain_expiry": {Type: CheckTypeDomainExpiry, DomainExpiry: &DomainExpiryCheck{Domain: "x.com", WarnDays: 30, CriticalDays: 7, Timeout: 5000}},
		"dns":           {Type: CheckTypeDNS, DNS: &DNSCheck{Domain: "x.com", RecordType: "A", Resolver: &resolver, Timeout: 5000}},
		"flow": {Type: CheckTypeFlow, Flow: &FlowCheck{
			StartURL: "https://app.example.com/login",
			Steps: []FlowStep{
				{Op: FlowOpFill, Selector: &fillSel, Value: "user"},
				{Op: FlowOpAssertURL, Contains: "/home"},
			},
			Timeout: 30000, StepTimeout: 5000, VerifyTLS: true,
		}},
	}
	for name, spec := range cases {
		t.Run(name, func(t *testing.T) {
			raw, err := json.Marshal(spec)
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}
			var m map[string]json.RawMessage
			if err := json.Unmarshal(raw, &m); err != nil {
				t.Fatalf("to map: %v", err)
			}
			if string(m["type"]) != `"`+name+`"` {
				t.Errorf("type = %s, want %q", m["type"], name)
			}
			var back CheckSpec
			if err := json.Unmarshal(raw, &back); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			if back.Type != spec.Type {
				t.Errorf("type round-trip = %q, want %q", back.Type, spec.Type)
			}
		})
	}
}

// TestFlowStep_TaggedPerOp pins that a flow step is internally tagged by "op"
// and carries only that op's fields, so the server never sees stray keys.
func TestFlowStep_TaggedPerOp(t *testing.T) {
	sel := "#user"
	cases := map[string]struct {
		step   FlowStep
		want   []string
		absent []string
	}{
		"goto":       {FlowStep{Op: FlowOpGoto, URL: "https://x/login"}, []string{"op", "url"}, []string{"selector", "value", "contains"}},
		"click":      {FlowStep{Op: FlowOpClick, Selector: &sel}, []string{"op", "selector"}, []string{"url", "value", "contains"}},
		"fill":       {FlowStep{Op: FlowOpFill, Selector: &sel, Value: "secret"}, []string{"op", "selector", "value"}, []string{"url", "contains"}},
		"wait_for":   {FlowStep{Op: FlowOpWaitFor, Selector: &sel}, []string{"op", "selector"}, []string{"url", "value", "contains"}},
		"assert_url": {FlowStep{Op: FlowOpAssertURL, Contains: "/home"}, []string{"op", "contains"}, []string{"url", "selector", "value"}},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			raw, err := json.Marshal(tc.step)
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}
			var m map[string]json.RawMessage
			if err := json.Unmarshal(raw, &m); err != nil {
				t.Fatalf("to map: %v", err)
			}
			for _, k := range tc.want {
				if _, ok := m[k]; !ok {
					t.Errorf("missing key %q in %s", k, raw)
				}
			}
			for _, k := range tc.absent {
				if _, ok := m[k]; ok {
					t.Errorf("unexpected key %q in %s", k, raw)
				}
			}
			var back FlowStep
			if err := json.Unmarshal(raw, &back); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			if back.Op != tc.step.Op {
				t.Errorf("op round-trip = %q, want %q", back.Op, tc.step.Op)
			}
		})
	}
}

// TestFlowStep_AssertTextNullSelector pins that a page-wide assert_text emits an
// explicit null selector and unmarshals back to nil.
func TestFlowStep_AssertTextNullSelector(t *testing.T) {
	raw, err := json.Marshal(FlowStep{Op: FlowOpAssertText, Contains: "Welcome"})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !strings.Contains(string(raw), `"selector":null`) {
		t.Errorf("assert_text with no selector must emit null, got %s", raw)
	}
	var back FlowStep
	if err := json.Unmarshal(raw, &back); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if back.Selector != nil {
		t.Errorf("selector = %q, want nil", *back.Selector)
	}
}

func TestChannelConfig_VariantsRoundTrip(t *testing.T) {
	cases := map[string]ChannelConfig{
		"webhook":     {Type: ChannelTypeWebhook, Webhook: &WebhookConfig{URL: "https://x", Headers: map[string]string{"A": "b"}}},
		"slack":       {Type: ChannelTypeSlack, Slack: &SlackConfig{WebhookURL: "https://hooks"}},
		"telegram":    {Type: ChannelTypeTelegram, Telegram: &TelegramConfig{BotToken: "123:abc", ChatID: "-100"}},
		"discord":     {Type: ChannelTypeDiscord, Discord: &DiscordConfig{WebhookURL: "https://discord.com/api/webhooks/1/x"}},
		"msteams":     {Type: ChannelTypeMsTeams, MsTeams: &MsTeamsConfig{WebhookURL: "https://contoso.webhook.office.com/x"}},
		"google_chat": {Type: ChannelTypeGoogleChat, GoogleChat: &GoogleChatConfig{WebhookURL: "https://chat.googleapis.com/v1/spaces/x"}},
		"email":       {Type: ChannelTypeEmail, Email: &EmailConfig{To: "oncall@example.com"}},
		"pagerduty":   {Type: ChannelTypePagerDuty, PagerDuty: &PagerDutyConfig{RoutingKey: "R0123456789abcdef0123456789abcde"}},
		"ntfy":        {Type: ChannelTypeNtfy, Ntfy: &NtfyConfig{ServerURL: "https://ntfy.sh", Topic: "uptime-alerts", AccessToken: "tk_x"}},
		"pushover":    {Type: ChannelTypePushover, Pushover: &PushoverConfig{Token: "azGDORePK8gMaC0QOYAMyEEuzJnyUi", User: "uQiRzpo4DXghDmr9QzzfQu27cmVRsG", Emergency: true}},
		"whatsapp":    {Type: ChannelTypeWhatsApp, WhatsApp: &WhatsAppConfig{AccessToken: "EAAG", PhoneNumberID: "123", To: "15551234567", TemplateName: "uptime_alert"}},
		"sms": {Type: ChannelTypeSMS, SMS: &SMSConfig{
			Provider: "twilio", To: "+15551234567", From: "+15557654321",
			AccountSID: "AC0123456789ABCDEF0123456789ABCDEF", AuthToken: "tok",
		}},
	}
	for name, cfg := range cases {
		t.Run(name, func(t *testing.T) {
			raw, err := json.Marshal(cfg)
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}
			var m map[string]json.RawMessage
			if err := json.Unmarshal(raw, &m); err != nil {
				t.Fatalf("to map: %v", err)
			}
			if string(m["type"]) != `"`+name+`"` {
				t.Errorf("type = %s, want %q", m["type"], name)
			}
			var back ChannelConfig
			if err := json.Unmarshal(raw, &back); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			if back.Type != cfg.Type {
				t.Errorf("type round-trip = %q, want %q", back.Type, cfg.Type)
			}
		})
	}
}

// TestChannelConfig_SMSFlatAndOmitsUnused pins that SMS is internally tagged
// (provider/to/from beside type, not nested) and that gateway fields the chosen
// provider doesn't use are omitted rather than sent empty.
func TestChannelConfig_SMSFlatAndOmitsUnused(t *testing.T) {
	cfg := ChannelConfig{Type: ChannelTypeSMS, SMS: &SMSConfig{
		Provider: "telnyx", To: "+15551234567", From: "alerts", APIKey: "KEY123",
	}}
	raw, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("to map: %v", err)
	}
	for _, k := range []string{"type", "provider", "to", "from", "api_key"} {
		if _, ok := m[k]; !ok {
			t.Errorf("missing flat key %q in %s", k, raw)
		}
	}
	for _, k := range []string{"account_sid", "auth_token", "api_secret", "auth_id", "service_plan_id", "api_token", "region", "messaging_profile_id"} {
		if _, ok := m[k]; ok {
			t.Errorf("unexpected key %q present for telnyx: %s", k, raw)
		}
	}
}

// TestCheckSpec_HTTPInternallyTagged pins that "type" is flattened alongside the
// http fields (internally tagged), not nested.
func TestCheckSpec_HTTPInternallyTagged(t *testing.T) {
	spec := CheckSpec{
		Type: CheckTypeHTTP,
		HTTP: &HTTPCheck{
			URL:            "https://example.com",
			Method:         "GET",
			Timeout:        5000,
			MaxRedirects:   5,
			ExpectedStatus: ExpectedStatus{Kind: StatusKindExact, Exact: 200},
			Headers:        map[string]string{},
			VerifyTLS:      true,
		},
	}
	raw, err := json.Marshal(spec)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("unmarshal to map: %v", err)
	}
	if string(m["type"]) != `"http"` {
		t.Errorf("type discriminator = %s, want \"http\" at top level", m["type"])
	}
	if _, ok := m["url"]; !ok {
		t.Error("url not flattened to top level (should be internally tagged)")
	}

	var back CheckSpec
	if err := json.Unmarshal(raw, &back); err != nil {
		t.Fatalf("unmarshal back: %v", err)
	}
	if back.Type != CheckTypeHTTP || back.HTTP == nil || back.HTTP.URL != "https://example.com" {
		t.Errorf("round-trip lost data: %+v", back)
	}
}

func TestChannelConfig_TelegramAppIsManaged(t *testing.T) {
	var c ChannelConfig
	err := json.Unmarshal([]byte(`{"type":"telegram_app","chat_id":"-100123"}`), &c)
	if err == nil {
		t.Fatal("telegram_app must not unmarshal into a manageable config")
	}
	if !strings.Contains(err.Error(), "cannot be managed by Terraform") {
		t.Fatalf("error should explain the managed kind, got: %v", err)
	}
}

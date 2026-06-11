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
	cases := map[string]CheckSpec{
		"tcp":           {Type: CheckTypeTCP, TCP: &TCPCheck{Host: "db", Port: 5432, Timeout: 3000}},
		"tls_cert":      {Type: CheckTypeTLSCert, TLSCert: &TLSCertCheck{Host: "x", Port: 443, WarnDays: 30, CriticalDays: 7, Timeout: 5000}},
		"domain_expiry": {Type: CheckTypeDomainExpiry, DomainExpiry: &DomainExpiryCheck{Domain: "x.com", WarnDays: 30, CriticalDays: 7, Timeout: 5000}},
		"dns":           {Type: CheckTypeDNS, DNS: &DNSCheck{Domain: "x.com", RecordType: "A", Resolver: &resolver, Timeout: 5000}},
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

func TestChannelConfig_VariantsRoundTrip(t *testing.T) {
	cases := map[string]ChannelConfig{
		"webhook":  {Type: ChannelTypeWebhook, Webhook: &WebhookConfig{URL: "https://x", Headers: map[string]string{"A": "b"}}},
		"slack":    {Type: ChannelTypeSlack, Slack: &SlackConfig{WebhookURL: "https://hooks"}},
		"telegram": {Type: ChannelTypeTelegram, Telegram: &TelegramConfig{BotToken: "123:abc", ChatID: "-100"}},
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

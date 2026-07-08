package client

import (
	"encoding/json"
	"fmt"
)

// Target is the read shape returned by GET/POST/PATCH /targets.
type Target struct {
	ID          string         `json:"id"`
	Name        string         `json:"name"`
	Check       CheckSpec      `json:"check"`
	Interval    uint64         `json:"interval"` // seconds
	Enabled     bool           `json:"enabled"`
	Tags        []string       `json:"tags"`
	Alerts      []AlertBinding `json:"alerts"`
	GroupName   *string        `json:"group_name"`
	OwnerUserID *string        `json:"owner_user_id"`
	CreatedAt   string         `json:"created_at,omitempty"`
	UpdatedAt   string         `json:"updated_at,omitempty"`
}

// NewTarget is the POST /targets body. Enabled has no omitempty: the zero value
// false must travel, otherwise the server applies its default-true. Tags/Alerts
// DO use omitempty: a nil slice would marshal to JSON null, which the server's
// serde-defaulted Vec rejects — omitting the key instead lets the default fire.
type NewTarget struct {
	Name        string         `json:"name"`
	Check       CheckSpec      `json:"check"`
	Interval    uint64         `json:"interval"`
	Enabled     bool           `json:"enabled"`
	Tags        []string       `json:"tags,omitempty"`
	Alerts      []AlertBinding `json:"alerts,omitempty"`
	GroupName   *string        `json:"group_name,omitempty"`
	OwnerUserID *string        `json:"owner_user_id,omitempty"`
}

// TargetUpdate is the PATCH /targets/{id} body. Terraform always holds the full
// desired state, so every field is sent on every update — no partial-patch
// bookkeeping. GroupName / OwnerUserID are pointers so a nil marshals to JSON
// null, which clears the field server-side (present-null = clear); a value
// sets it. Tags/Alerts must be non-nil (UpdateTarget normalizes) so an empty
// slice clears rather than a null being misread as "keep".
type TargetUpdate struct {
	Name        string         `json:"name"`
	Check       CheckSpec      `json:"check"`
	Interval    uint64         `json:"interval"`
	Enabled     bool           `json:"enabled"`
	Tags        []string       `json:"tags"`
	Alerts      []AlertBinding `json:"alerts"`
	GroupName   *string        `json:"group_name"`
	OwnerUserID *string        `json:"owner_user_id"`
}

// TargetRegions is the GET/PUT /targets/{id}/regions body. Regions live on this
// sub-resource, not on the target itself: POST /targets auto-assigns the full
// set (up to the plan cap), and PUT replaces the set wholesale. The server
// rejects an empty list (>= 1 region required) and unknown/disabled region ids
// with 422 REGION_INVALID.
type TargetRegions struct {
	Regions []string `json:"regions"`
}

// AlertBinding ties a notification channel to a target's failure threshold.
type AlertBinding struct {
	ChannelID      string `json:"channel_id"`
	AfterFailures  uint32 `json:"after_failures"`
	NotifyRecovery bool   `json:"notify_recovery"`
}

// HTTPCheck is the http variant of CheckSpec. basic_auth and bearer_token come
// back as "***" on read; the provider keeps prior state for those.
type HTTPCheck struct {
	URL                  string            `json:"url"`
	Method               string            `json:"method"`  // UPPERCASE
	Timeout              uint64            `json:"timeout"` // milliseconds
	FollowRedirects      bool              `json:"follow_redirects"`
	MaxRedirects         uint8             `json:"max_redirects"`
	ExpectedStatus       ExpectedStatus    `json:"expected_status"`
	ExpectedBodyContains *string           `json:"expected_body_contains"`
	Headers              map[string]string `json:"headers"` // required: must marshal as {} not null
	Body                 *string           `json:"body"`
	VerifyTLS            bool              `json:"verify_tls"`
	BasicAuth            *[2]string        `json:"basic_auth"`   // ["user","pass"]
	BearerToken          *string           `json:"bearer_token"` // "***" on read if set
}

// TCPCheck is the tcp variant: a connect probe to host:port.
type TCPCheck struct {
	Host    string `json:"host"`
	Port    uint16 `json:"port"`
	Timeout uint64 `json:"timeout"` // milliseconds
}

// TLSCertCheck verifies a certificate and warns/fails on imminent expiry.
type TLSCertCheck struct {
	Host         string  `json:"host"`
	Port         uint16  `json:"port"`
	ServerName   *string `json:"server_name"`
	WarnDays     uint32  `json:"warn_days"`
	CriticalDays uint32  `json:"critical_days"`
	Timeout      uint64  `json:"timeout"` // milliseconds
}

// DomainExpiryCheck warns/fails as a domain registration nears expiry.
type DomainExpiryCheck struct {
	Domain       string `json:"domain"`
	WarnDays     uint32 `json:"warn_days"`
	CriticalDays uint32 `json:"critical_days"`
	Timeout      uint64 `json:"timeout"` // milliseconds
}

// DNSCheck resolves a name and optionally asserts a substring in the answers.
type DNSCheck struct {
	Domain           string  `json:"domain"`
	RecordType       string  `json:"record_type"` // A, AAAA, CNAME, MX, NS, TXT, SOA, PTR, CAA, SRV
	Resolver         *string `json:"resolver"`
	ExpectedContains *string `json:"expected_contains"`
	Timeout          uint64  `json:"timeout"` // milliseconds
}

// FlowCheck drives a headless browser through Steps starting at StartURL, so it
// verifies a login/transaction session rather than a single request.
type FlowCheck struct {
	StartURL    string     `json:"start_url"`
	Steps       []FlowStep `json:"steps"`
	Timeout     uint64     `json:"timeout"`      // milliseconds
	StepTimeout uint64     `json:"step_timeout"` // milliseconds
	VerifyTLS   bool       `json:"verify_tls"`
}

// FlowStep is one action in a FlowCheck, internally tagged by "op" with only the
// fields that op uses. A fill value comes back "***" on read; the provider keeps
// prior state for it.
type FlowStep struct {
	Op       string
	URL      string  // goto
	Selector *string // click / fill / wait_for; optional for assert_text
	Value    string  // fill
	Contains string  // assert_text / assert_url
}

const (
	FlowOpGoto       = "goto"
	FlowOpClick      = "click"
	FlowOpFill       = "fill"
	FlowOpWaitFor    = "wait_for"
	FlowOpAssertText = "assert_text"
	FlowOpAssertURL  = "assert_url"
)

func (s FlowStep) MarshalJSON() ([]byte, error) {
	switch s.Op {
	case FlowOpGoto:
		return json.Marshal(struct {
			Op  string `json:"op"`
			URL string `json:"url"`
		}{s.Op, s.URL})
	case FlowOpClick, FlowOpWaitFor:
		return json.Marshal(struct {
			Op       string `json:"op"`
			Selector string `json:"selector"`
		}{s.Op, strDeref(s.Selector)})
	case FlowOpFill:
		return json.Marshal(struct {
			Op       string `json:"op"`
			Selector string `json:"selector"`
			Value    string `json:"value"`
		}{s.Op, strDeref(s.Selector), s.Value})
	case FlowOpAssertText:
		// selector is nullable: null asserts against the whole page.
		return json.Marshal(struct {
			Op       string  `json:"op"`
			Selector *string `json:"selector"`
			Contains string  `json:"contains"`
		}{s.Op, s.Selector, s.Contains})
	case FlowOpAssertURL:
		return json.Marshal(struct {
			Op       string `json:"op"`
			Contains string `json:"contains"`
		}{s.Op, s.Contains})
	case "":
		return nil, fmt.Errorf("flow step has no op")
	default:
		return nil, fmt.Errorf("unsupported flow step op %q", s.Op)
	}
}

func (s *FlowStep) UnmarshalJSON(data []byte) error {
	var wire struct {
		Op       string  `json:"op"`
		URL      string  `json:"url"`
		Selector *string `json:"selector"`
		Value    string  `json:"value"`
		Contains string  `json:"contains"`
	}
	if err := json.Unmarshal(data, &wire); err != nil {
		return err
	}
	switch wire.Op {
	case FlowOpGoto, FlowOpClick, FlowOpFill, FlowOpWaitFor, FlowOpAssertText, FlowOpAssertURL:
		*s = FlowStep{Op: wire.Op, URL: wire.URL, Selector: wire.Selector, Value: wire.Value, Contains: wire.Contains}
		return nil
	default:
		return fmt.Errorf("unsupported flow step op %q", wire.Op)
	}
}

func strDeref(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

// CheckSpec is the internally-tagged union over check kinds (discriminator
// "type", variant fields flattened alongside it). Exactly one variant pointer
// is set, matching Type.
type CheckSpec struct {
	Type         string             `json:"-"`
	HTTP         *HTTPCheck         `json:"-"`
	TCP          *TCPCheck          `json:"-"`
	TLSCert      *TLSCertCheck      `json:"-"`
	DomainExpiry *DomainExpiryCheck `json:"-"`
	DNS          *DNSCheck          `json:"-"`
	Flow         *FlowCheck         `json:"-"`
}

const (
	CheckTypeHTTP         = "http"
	CheckTypeTCP          = "tcp"
	CheckTypeTLSCert      = "tls_cert"
	CheckTypeDomainExpiry = "domain_expiry"
	CheckTypeDNS          = "dns"
	CheckTypeFlow         = "flow"
)

func (c CheckSpec) MarshalJSON() ([]byte, error) {
	switch c.Type {
	case CheckTypeHTTP:
		if c.HTTP == nil {
			return nil, errNilPayload(c.Type)
		}
		h := *c.HTTP
		if h.Headers == nil {
			h.Headers = map[string]string{} // server requires the key; rejects null
		}
		// Embedding flattens the variant fields alongside the discriminator
		// (internally-tagged encoding) in a single pass, deterministic order.
		return json.Marshal(struct {
			Type string `json:"type"`
			HTTPCheck
		}{c.Type, h})
	case CheckTypeTCP:
		if c.TCP == nil {
			return nil, errNilPayload(c.Type)
		}
		return json.Marshal(struct {
			Type string `json:"type"`
			TCPCheck
		}{c.Type, *c.TCP})
	case CheckTypeTLSCert:
		if c.TLSCert == nil {
			return nil, errNilPayload(c.Type)
		}
		return json.Marshal(struct {
			Type string `json:"type"`
			TLSCertCheck
		}{c.Type, *c.TLSCert})
	case CheckTypeDomainExpiry:
		if c.DomainExpiry == nil {
			return nil, errNilPayload(c.Type)
		}
		return json.Marshal(struct {
			Type string `json:"type"`
			DomainExpiryCheck
		}{c.Type, *c.DomainExpiry})
	case CheckTypeDNS:
		if c.DNS == nil {
			return nil, errNilPayload(c.Type)
		}
		return json.Marshal(struct {
			Type string `json:"type"`
			DNSCheck
		}{c.Type, *c.DNS})
	case CheckTypeFlow:
		if c.Flow == nil {
			return nil, errNilPayload(c.Type)
		}
		f := *c.Flow
		if f.Steps == nil {
			f.Steps = []FlowStep{} // server requires the key; rejects null
		}
		return json.Marshal(struct {
			Type string `json:"type"`
			FlowCheck
		}{c.Type, f})
	case "":
		return nil, fmt.Errorf("check has no type")
	default:
		return nil, fmt.Errorf("unsupported check type %q", c.Type)
	}
}

func errNilPayload(t string) error {
	return fmt.Errorf("check type %q with nil payload", t)
}

func (c *CheckSpec) UnmarshalJSON(data []byte) error {
	*c = CheckSpec{} // clear any stale variant when the destination is reused
	var probe struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(data, &probe); err != nil {
		return err
	}
	c.Type = probe.Type
	switch probe.Type {
	case CheckTypeHTTP:
		c.HTTP = new(HTTPCheck)
		return json.Unmarshal(data, c.HTTP)
	case CheckTypeTCP:
		c.TCP = new(TCPCheck)
		return json.Unmarshal(data, c.TCP)
	case CheckTypeTLSCert:
		c.TLSCert = new(TLSCertCheck)
		return json.Unmarshal(data, c.TLSCert)
	case CheckTypeDomainExpiry:
		c.DomainExpiry = new(DomainExpiryCheck)
		return json.Unmarshal(data, c.DomainExpiry)
	case CheckTypeDNS:
		c.DNS = new(DNSCheck)
		return json.Unmarshal(data, c.DNS)
	case CheckTypeFlow:
		c.Flow = new(FlowCheck)
		return json.Unmarshal(data, c.Flow)
	default:
		return fmt.Errorf("unsupported check type %q", probe.Type)
	}
}

// ExpectedStatus is adjacently tagged: {"kind":..,"value":..}. The variant
// payload always sits under "value" (range does NOT flatten min/max).
type ExpectedStatus struct {
	Kind  string       // exact | range | one_of
	Exact uint16       // when Kind == exact
	Range *StatusRange // when Kind == range
	OneOf []uint16     // when Kind == one_of
}

type StatusRange struct {
	Min uint16 `json:"min"`
	Max uint16 `json:"max"`
}

const (
	StatusKindExact = "exact"
	StatusKindRange = "range"
	StatusKindOneOf = "one_of"
)

func (e ExpectedStatus) MarshalJSON() ([]byte, error) {
	wire := struct {
		Kind  string `json:"kind"`
		Value any    `json:"value"`
	}{Kind: e.Kind}
	switch e.Kind {
	case StatusKindExact:
		wire.Value = e.Exact
	case StatusKindRange:
		if e.Range == nil {
			return nil, fmt.Errorf("expected_status range with nil bounds")
		}
		wire.Value = e.Range
	case StatusKindOneOf:
		wire.Value = e.OneOf
	default:
		return nil, fmt.Errorf("unsupported expected_status kind %q", e.Kind)
	}
	return json.Marshal(wire)
}

func (e *ExpectedStatus) UnmarshalJSON(data []byte) error {
	var wire struct {
		Kind  string          `json:"kind"`
		Value json.RawMessage `json:"value"`
	}
	if err := json.Unmarshal(data, &wire); err != nil {
		return err
	}
	e.Kind = wire.Kind
	switch wire.Kind {
	case StatusKindExact:
		return json.Unmarshal(wire.Value, &e.Exact)
	case StatusKindRange:
		var r StatusRange
		if err := json.Unmarshal(wire.Value, &r); err != nil {
			return err
		}
		e.Range = &r
		return nil
	case StatusKindOneOf:
		return json.Unmarshal(wire.Value, &e.OneOf)
	default:
		return fmt.Errorf("unsupported expected_status kind %q", wire.Kind)
	}
}

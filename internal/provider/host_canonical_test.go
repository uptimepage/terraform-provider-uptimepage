package provider

import (
	"context"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// canonicalHostCases mirrors the corpus the API pins against its own
// canonicalize_check, entry for entry. `stored` is what the API writes, taken
// from that corpus, so a case marked predictable here and answered differently
// there is a real divergence rather than a disagreement about intent.
//
// hostUnpredictable is the ASCII-subset boundary: those rows say what the API
// stores, and that this provider deliberately declines to predict it.
var canonicalHostCases = []struct {
	in     string
	want   hostVerdict
	stored string
	why    string
}{
	{"example.com", hostCanonical, "example.com", "already canonical"},
	{"sub.example.co.uk", hostCanonical, "sub.example.co.uk", "multi-label"},
	{"db", hostCanonical, "db", "single-label internal host"},
	{"localhost", hostCanonical, "localhost", "localhost"},
	{"my-svc", hostCanonical, "my-svc", "interior hyphen is ordinary"},
	{"1abc.com", hostCanonical, "1abc.com", "digit-leading label"},
	{"EXAMPLE.com", hostRewritten, "example.com", "ascii case folded"},
	{"Example.COM.", hostRewritten, "example.com", "case and trailing dot"},
	{"example.com.", hostRewritten, "example.com", "trailing dot stripped"},
	{"example.com..", hostRewritten, "example.com", "repeated trailing dots"},
	{"1.2.3.4", hostCanonical, "1.2.3.4", "ipv4 bypasses idn"},
	{"1.2.3.4.", hostRewritten, "1.2.3.4", "a trailing dot stops it parsing as an ip, so idn trims it"},
	{"2001:db8::1", hostCanonical, "2001:db8::1", "ipv6 already canonical"},
	{"2001:DB8::1", hostRewritten, "2001:db8::1", "ipv6 case folded"},
	{"2001:db8:0:0::1", hostRewritten, "2001:db8::1", "ipv6 zero run compressed"},
	{"2001:0db8:0000:0000:0000:0000:0000:0001", hostRewritten, "2001:db8::1", "ipv6 fully expanded"},
	// net.ParseIP collapses this to 1.2.3.4 where the API keeps the prefix,
	// so predicting it would name a different address.
	{"::FFFF:1.2.3.4", hostUnpredictable, "::ffff:1.2.3.4", "v4-mapped, where the two disagree"},
	{"1.2.3.04", hostCanonical, "1.2.3.04", "a leading zero stops it being an ip"},
	{"[2001:db8::1]", hostRewritten, "2001:db8::1", "bracketed ipv6 unbracketed"},
	{"[example.com]", hostRewritten, "example.com", "brackets come off a name too"},
	{"[oops", hostRejected, "", "one bracket is not a pair"},
	{"--invalid-leading.com", hostRejected, "", "leading hyphens"},
	{"-lead.com", hostRejected, "", "leading hyphen"},
	{"trail-.com", hostRejected, "", "trailing hyphen"},
	{"ab--cd.com", hostRejected, "", "hyphen pair in the third and fourth position"},
	{"under_score.com", hostRejected, "", "underscore is not in the std3 set"},
	{"a..b.com", hostRejected, "", "empty interior label"},
	{"exa mple.com", hostRejected, "", "space"},
	{"example.com:8080", hostRejected, "", "port is not part of the host"},
	{"-", hostRejected, "", "bare hyphen"},
	{"", hostRejected, "", "empty host"},
	{"Bähn.de", hostUnpredictable, "xn--bhn-qla.de", "non-ascii"},
	{"BÄHN.de", hostUnpredictable, "xn--bhn-qla.de", "uppercase non-ascii"},
	{"bähn.de.", hostUnpredictable, "xn--bhn-qla.de", "non-ascii plus trailing dot"},
	{"приклад.укр", hostUnpredictable, "xn--80aikifvh.xn--j1amh", "cyrillic"},
	{"xn--bhn-qla.de", hostUnpredictable, "xn--bhn-qla.de", "punycode label decodes to unicode"},
	{"xn--dh0dc.com", hostUnpredictable, "xn--dh0dc.com", "punycode the two uts46 tables disagree on"},
}

func TestClassifyHostMatchesTheAPICorpus(t *testing.T) {
	for _, c := range canonicalHostCases {
		t.Run(c.why, func(t *testing.T) {
			got, canon := classifyHost(c.in)
			if got != c.want {
				t.Fatalf("classifyHost(%q) = %v, want %v", c.in, got, c.want)
			}
			switch got {
			case hostCanonical, hostRewritten:
				if canon != c.stored {
					t.Errorf("classifyHost(%q) = %q, the API stores %q", c.in, canon, c.stored)
				}
			case hostRejected:
				// The second value carries the rule that was broken.
				if canon == "" {
					t.Errorf("classifyHost(%q) rejects without saying why", c.in)
				}
			case hostUnpredictable:
				if canon != "" {
					t.Errorf("classifyHost(%q) returned %q while deferring", c.in, canon)
				}
			}
		})
	}
}

// A hostRewritten verdict claiming the input is already canonical, or the
// reverse, would produce an error message telling someone to write what they
// already wrote.
func TestCanonicalAndRewrittenAreNotConfused(t *testing.T) {
	for _, c := range canonicalHostCases {
		switch c.want {
		case hostCanonical:
			if c.stored != c.in {
				t.Errorf("%q is marked canonical but the API stores %q", c.in, c.stored)
			}
		case hostRewritten:
			if c.stored == c.in {
				t.Errorf("%q is marked rewritten but the API stores it unchanged", c.in)
			}
		}
	}
}

// The limits are hard-coded here, so they are worth their own case: the API
// pins the same four in its suite.
func TestDNSLengthLimits(t *testing.T) {
	label := strings.Repeat("a", 63)
	for _, c := range []struct {
		in   string
		want hostVerdict
		why  string
	}{
		{label + ".example.com", hostCanonical, "63-character label"},
		{"a" + label + ".example.com", hostRejected, "64-character label"},
		{label + "." + label + "." + label + "." + strings.Repeat("a", 61), hostCanonical, "253-character host"},
		{label + "." + label + "." + label + "." + strings.Repeat("a", 62), hostRejected, "254-character host"},
	} {
		t.Run(c.why, func(t *testing.T) {
			if got, _ := classifyHost(c.in); got != c.want {
				t.Errorf("classifyHost(%s) = %v, want %v", c.why, got, c.want)
			}
		})
	}
}

func validateHost(t *testing.T, in string) []string {
	t.Helper()
	resp := &validator.StringResponse{}
	canonicalHostValidator("check.host").ValidateString(context.Background(), validator.StringRequest{
		Path:        path.Root("check").AtName("ping").AtName("host"),
		ConfigValue: types.StringValue(in),
	}, resp)
	var out []string
	for _, d := range resp.Diagnostics.Errors() {
		out = append(out, d.Summary())
	}
	return out
}

func TestTheValidatorFlagsOnlyThePredictableFailures(t *testing.T) {
	for _, c := range canonicalHostCases {
		t.Run(c.why, func(t *testing.T) {
			errs := validateHost(t, c.in)
			switch c.want {
			case hostCanonical:
				if len(errs) > 0 {
					t.Errorf("%q is canonical but was rejected: %v", c.in, errs)
				}
			case hostUnpredictable:
				// Deliberately silent: the apply reports these.
				if len(errs) > 0 {
					t.Errorf("%q is outside the ASCII subset but was rejected: %v", c.in, errs)
				}
			case hostRewritten, hostRejected:
				if len(errs) == 0 {
					t.Errorf("%q would fail the apply but the plan accepted it", c.in)
				}
			}
		})
	}
}

// The message has to carry the string to write. Without it someone knows the
// host is wrong and not what to put instead, which is the apply error again.
func TestTheRewriteMessageNamesTheStringToWrite(t *testing.T) {
	resp := &validator.StringResponse{}
	canonicalHostValidator("check.host").ValidateString(context.Background(), validator.StringRequest{
		Path:        path.Root("check").AtName("ping").AtName("host"),
		ConfigValue: types.StringValue("EXAMPLE.com"),
	}, resp)
	errs := resp.Diagnostics.Errors()
	if len(errs) != 1 {
		t.Fatalf("got %d diagnostics, want 1", len(errs))
	}
	if !strings.Contains(errs[0].Detail(), "example.com") {
		t.Errorf("message does not name the canonical host: %q", errs[0].Detail())
	}
}

// A null or unknown value belongs to the framework's Required check and to
// apply time respectively; flagging either would fail plans that are fine.
func TestValidatorIgnoresNullAndUnknown(t *testing.T) {
	for name, v := range map[string]types.String{
		"null":    types.StringNull(),
		"unknown": types.StringUnknown(),
	} {
		t.Run(name, func(t *testing.T) {
			resp := &validator.StringResponse{}
			canonicalHostValidator("check.host").ValidateString(context.Background(),
				validator.StringRequest{Path: path.Root("check"), ConfigValue: v}, resp)
			if resp.Diagnostics.HasError() {
				t.Errorf("%s value rejected: %v", name, resp.Diagnostics.Errors())
			}
		})
	}
}

// Every host-shaped attribute must carry the validator. A new check kind that
// adds one without it reintroduces the apply-time failure this replaced, and
// nothing else in the suite would notice.
func TestEveryHostAttributeIsCanonicalValidated(t *testing.T) {
	var resp resource.SchemaResponse
	(&targetResource{}).Schema(context.Background(), resource.SchemaRequest{}, &resp)
	check, ok := resp.Schema.Attributes["check"].(schema.SingleNestedAttribute)
	if !ok {
		t.Fatal("check is not a single nested attribute")
	}
	walked := 0
	for kind, attr := range check.Attributes {
		nested, ok := attr.(schema.SingleNestedAttribute)
		if !ok {
			continue
		}
		for name, a := range nested.Attributes {
			if name != "host" && name != "domain" {
				continue
			}
			walked++
			sa, ok := a.(schema.StringAttribute)
			if !ok {
				t.Errorf("check.%s.%s is not a string attribute", kind, name)
				continue
			}
			guarded := false
			for _, v := range sa.Validators {
				if _, ok := v.(canonicalHostValidatorImpl); ok {
					guarded = true
				}
			}
			if !guarded {
				t.Errorf("check.%s.%s accepts a host the API would rewrite: no canonical validator", kind, name)
			}
		}
	}
	// tcp.host, ping.host, tls_cert.host, domain_expiry.domain, dns.domain.
	if walked != 5 {
		t.Errorf("walked %d host/domain attributes, want 5", walked)
	}
}

// Every rule that rejects needs its own message. One message covering the
// character set told a user with a 64-character label about letters and
// digits, which does not describe what they hit.
func TestEachRejectionRuleExplainsItself(t *testing.T) {
	label := strings.Repeat("a", 64)
	cases := []struct {
		in   string
		want string
		why  string
	}{
		{"", "cannot be empty", "empty host"},
		{strings.Repeat("a.", 127) + "aa", "at most 253 characters", "host too long"},
		{label + ".com", "at most 63 characters", "label too long"},
		{"a..b.com", "empty label", "doubled dot"},
		{"-lead.com", "begin or end with a hyphen", "leading hyphen"},
		{"trail-.com", "begin or end with a hyphen", "trailing hyphen"},
		{"ab--cd.com", "third and fourth position", "hyphen pair"},
		{"under_score.com", "letters, digits and hyphens", "bad character"},
	}
	seen := map[string]string{}
	for _, c := range cases {
		t.Run(c.why, func(t *testing.T) {
			got, reason := classifyHost(c.in)
			if got != hostRejected {
				t.Fatalf("classifyHost(%q) = %v, want hostRejected", c.in, got)
			}
			if !strings.Contains(reason, c.want) {
				t.Errorf("classifyHost(%q) says %q, want it to mention %q", c.in, reason, c.want)
			}
			// A rule that reuses another's wording is the bug this guards.
			if prev, dup := seen[c.want]; dup && prev != c.why {
				t.Logf("shared wording with %s, which is intended here", prev)
			}
			seen[c.want] = c.why
		})
	}
}

// The message a user reads has to carry the reason, not just the host.
func TestTheRejectMessageCarriesTheReason(t *testing.T) {
	errs := validateHost(t, strings.Repeat("a", 64)+".com")
	if len(errs) != 1 {
		t.Fatalf("got %d diagnostics, want 1", len(errs))
	}
	resp := &validator.StringResponse{}
	canonicalHostValidator("check.host").ValidateString(context.Background(), validator.StringRequest{
		Path:        path.Root("check"),
		ConfigValue: types.StringValue(strings.Repeat("a", 64) + ".com"),
	}, resp)
	detail := resp.Diagnostics.Errors()[0].Detail()
	if !strings.Contains(detail, "63 characters") {
		t.Errorf("message does not name the rule that was broken: %q", detail)
	}
}

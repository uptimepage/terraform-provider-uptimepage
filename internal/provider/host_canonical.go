package provider

import (
	"context"
	"fmt"
	"net"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
)

type hostVerdict int

const (
	// hostUnpredictable: outside the ASCII subset, so only the API can say.
	hostUnpredictable hostVerdict = iota
	// hostCanonical: the API stores this string unchanged.
	hostCanonical
	// hostRewritten: the API stores a different string, so the apply fails.
	hostRewritten
	// hostRejected: the API answers 400.
	hostRejected
)

// The API rewrites hosts on write and stores the canonical form, because the
// circuit breaker, the per-(org, host, port) throttle and the per-TLD RDAP
// throttle all key on it: without that, changing the case of a hostname buys a
// fresh breaker and a fresh rate-limit bucket. Terraform requires a Required
// attribute to read back exactly as configured, so a host the API rewrites
// fails the apply with "provider produced inconsistent result after apply".
//
// This validator moves the common cases to plan time, where the message can
// name the string to write instead of leaving a stuck apply.
//
// It covers only plain-ASCII hosts. The API canonicalises with UTS46, and no
// second implementation of UTS46 agrees with the first: the Unicode mapping
// tables differ by version, and the deviation characters are mapped by some
// implementations and passed through by others. A validator that guessed wrong
// would tell someone to write a hostname that resolves somewhere else, which
// is worse than the apply error it replaces. So anything holding a non-ASCII
// character or an xn-- label is left to the API, and the apply error is what
// those hit. That subset is rare in practice and the escape hatch is to write
// the punycode form.
//
// canonicalHostCases is asserted against the same corpus the API pins in its
// own suite.
//
// classifyHost mirrors the API's canonicalize_check for plain-ASCII hosts. The
// second value is the form the API would store, or for hostRejected the rule
// that was broken, so the message can describe the actual failure.
func classifyHost(raw string) (hostVerdict, string) {
	// Brackets come off first and unconditionally, matching the API: a
	// bracketed pair is stripped whether or not what it wraps is an IP, so
	// "[example.com]" is stored as "example.com". One lone bracket is not a
	// pair and stays part of the host, where the character check rejects it.
	host := raw
	if inner, ok := strings.CutPrefix(host, "["); ok {
		if unwrapped, ok := strings.CutSuffix(inner, "]"); ok {
			host = unwrapped
		}
	}

	// An IP takes no IDN processing, but the API does store the parsed
	// address, so "2001:db8:0:0::1" comes back as "2001:db8::1".
	if ip := net.ParseIP(host); ip != nil {
		// net.ParseIP collapses a v4-mapped address to its dotted form, where
		// the API keeps the ::ffff: prefix. Predicting from this would name a
		// different address, so leave those to the apply.
		if ip.To4() != nil && strings.Contains(host, ":") {
			return hostUnpredictable, ""
		}
		return hostVerdictFor(raw, ip.String())
	}

	host = strings.TrimRight(host, ".")
	if !isASCII(host) || hasPunycodeLabel(host) {
		return hostUnpredictable, ""
	}
	host = strings.ToLower(host)
	if problem := asciiDomainProblem(host); problem != "" {
		return hostRejected, problem
	}
	return hostVerdictFor(raw, host)
}

func hostVerdictFor(raw, canon string) (hostVerdict, string) {
	if canon == raw {
		return hostCanonical, canon
	}
	return hostRewritten, canon
}

func isASCII(s string) bool {
	for i := range len(s) {
		if s[i] > 0x7f {
			return false
		}
	}
	return true
}

// A punycode label decodes to Unicode, which puts it back in the territory the
// two UTS46 implementations disagree about.
func hasPunycodeLabel(host string) bool {
	for _, label := range strings.Split(host, ".") {
		if len(label) >= 4 && strings.EqualFold(label[:4], "xn--") {
			return true
		}
	}
	return false
}

// UTS46 ToASCII with UseSTD3ASCIIRules, CheckHyphens and VerifyDNSLength, for
// the ASCII subset. Returns the rule that was broken, or "" when the host is
// valid: five different rules reject here, and one message covering only the
// character set leaves the other four looking like a bug in the provider.
func asciiDomainProblem(host string) string {
	switch {
	case len(host) == 0:
		return "a host cannot be empty"
	case len(host) > 253:
		return fmt.Sprintf("a host may be at most 253 characters, this one is %d", len(host))
	}
	for _, label := range strings.Split(host, ".") {
		switch {
		case len(label) == 0:
			return "a host cannot hold an empty label, so no leading or doubled dot"
		case len(label) > 63:
			return fmt.Sprintf("a label may be at most 63 characters, %q is %d", label, len(label))
		case label[0] == '-' || label[len(label)-1] == '-':
			return fmt.Sprintf("a label may not begin or end with a hyphen, but %q does", label)
		// CheckHyphens rejects a pair in both positions, which is what marks
		// an A-label prefix. A single hyphen there is ordinary.
		case len(label) >= 4 && label[2] == '-' && label[3] == '-':
			return fmt.Sprintf("a label may not hold hyphens in both the third and fourth position, but %q does", label)
		}
		for i := range len(label) {
			c := label[i]
			ldh := c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '-'
			if !ldh {
				return fmt.Sprintf("a label may hold only letters, digits and hyphens, but %q holds %q", label, string(c))
			}
		}
	}
	return ""
}

type canonicalHostValidatorImpl struct{ attr string }

func (v canonicalHostValidatorImpl) Description(context.Context) string {
	return "host must be in the canonical form the API stores; checked for plain-ASCII hosts only"
}

func (v canonicalHostValidatorImpl) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

func (v canonicalHostValidatorImpl) ValidateString(ctx context.Context, req validator.StringRequest, resp *validator.StringResponse) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}
	raw := req.ConfigValue.ValueString()
	switch got, canon := classifyHost(raw); got {
	case hostRejected:
		resp.Diagnostics.AddAttributeError(req.Path, "Invalid host",
			fmt.Sprintf("%q is not a valid domain or IP address: %s.", raw, canon))
	case hostRewritten:
		resp.Diagnostics.AddAttributeError(req.Path, "Non-canonical host",
			fmt.Sprintf("The API stores %s in canonical form, and the rate throttle "+
				"and circuit breaker key on it, so %q would be written back as %q and "+
				"fail the apply. Write %q.", v.attr, raw, canon, canon))
	}
}

// canonicalHostValidator rejects at plan time what the API would rewrite.
func canonicalHostValidator(attr string) validator.String {
	return canonicalHostValidatorImpl{attr: attr}
}

package provider

import (
	"net/url"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/types"
)

// canonicalizeURL mirrors the server's `url::Url` canonicalization — lowercased
// scheme and host, and a "/" path when the path is empty — so a user-supplied
// URL can be compared against the form the API stores and echoes back.
func canonicalizeURL(s string) string {
	u, err := url.Parse(s)
	if err != nil {
		return s
	}
	u.Scheme = strings.ToLower(u.Scheme)
	u.Host = strings.ToLower(u.Host)
	if u.Path == "" {
		u.Path = "/"
	}
	return u.String()
}

// keepURL preserves the user's URL form in state when it canonicalizes to the
// value the server returns. The API canonicalizes URLs (e.g.
// "https://x.com" -> "https://x.com/"); echoing that back would trip
// Terraform's post-apply consistency check and show a perpetual diff. A
// server value that is NOT a mere canonicalization of the prior (real
// out-of-band drift) is surfaced as-is. Mirrors the keepSecret pattern.
func keepURL(prior types.String, server string) types.String {
	if !prior.IsNull() && !prior.IsUnknown() && canonicalizeURL(prior.ValueString()) == server {
		return prior
	}
	return types.StringValue(server)
}

package provider

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestCanonicalizeURL(t *testing.T) {
	cases := map[string]string{
		"https://github.com":         "https://github.com/",
		"https://www.google.com":     "https://www.google.com/",
		"HTTPS://Example.COM":        "https://example.com/",
		"https://EXAMPLE.com/a/b":    "https://example.com/a/b",
		"https://x.com/already/path": "https://x.com/already/path",
	}
	for in, want := range cases {
		if got := canonicalizeURL(in); got != want {
			t.Errorf("canonicalizeURL(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestKeepURL(t *testing.T) {
	// Prior canonicalizes to the server value -> keep the user's form.
	if got := keepURL(types.StringValue("https://github.com"), "https://github.com/"); got.ValueString() != "https://github.com" {
		t.Errorf("equivalent prior: got %q, want it kept as https://github.com", got.ValueString())
	}
	// Case-only difference is still equivalent -> keep the user's form.
	if got := keepURL(types.StringValue("https://GitHub.com"), "https://github.com/"); got.ValueString() != "https://GitHub.com" {
		t.Errorf("case-equivalent prior: got %q, want it kept", got.ValueString())
	}
	// Genuinely different server value (real drift) -> surface it.
	if got := keepURL(types.StringValue("https://old.com/"), "https://new.com/"); got.ValueString() != "https://new.com/" {
		t.Errorf("drift: got %q, want https://new.com/", got.ValueString())
	}
	// No prior (null) -> use the server value.
	if got := keepURL(types.StringNull(), "https://x.com/"); got.ValueString() != "https://x.com/" {
		t.Errorf("null prior: got %q, want https://x.com/", got.ValueString())
	}
}

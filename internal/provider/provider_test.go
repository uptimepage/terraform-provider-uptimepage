package provider

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/provider"
)

func TestProvider_Metadata(t *testing.T) {
	p := New("1.2.3")()
	var resp provider.MetadataResponse
	p.Metadata(context.Background(), provider.MetadataRequest{}, &resp)
	if resp.TypeName != "uptimepage" {
		t.Errorf("TypeName = %q, want uptimepage", resp.TypeName)
	}
	if resp.Version != "1.2.3" {
		t.Errorf("Version = %q, want 1.2.3", resp.Version)
	}
}

func TestProvider_Schema(t *testing.T) {
	p := New("test")()
	var resp provider.SchemaResponse
	p.Schema(context.Background(), provider.SchemaRequest{}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("schema diagnostics: %v", resp.Diagnostics)
	}
	for _, name := range []string{"endpoint", "token"} {
		if _, ok := resp.Schema.Attributes[name]; !ok {
			t.Errorf("schema missing attribute %q", name)
		}
	}
	if !resp.Schema.Attributes["token"].IsSensitive() {
		t.Error("token attribute must be sensitive")
	}
}

func TestResolveSettings_Precedence(t *testing.T) {
	cases := []struct {
		name                                    string
		cfgEndpoint, cfgToken, envEndpt, envTok string
		wantEndpoint, wantToken                 string
	}{
		{"config wins", "https://cfg", "tok-cfg", "https://env", "tok-env", "https://cfg", "tok-cfg"},
		{"env fallback", "", "", "https://env", "tok-env", "https://env", "tok-env"},
		{"unset endpoint passes through empty", "", "tok", "", "", "", "tok"},
		{"config over env per-field", "https://cfg", "", "https://env", "tok-env", "https://cfg", "tok-env"},
		{"no token anywhere", "", "", "", "", "", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ep, tok := resolveSettings(tc.cfgEndpoint, tc.cfgToken, tc.envEndpt, tc.envTok)
			if ep != tc.wantEndpoint {
				t.Errorf("endpoint = %q, want %q", ep, tc.wantEndpoint)
			}
			if tok != tc.wantToken {
				t.Errorf("token = %q, want %q", tok, tc.wantToken)
			}
		})
	}
}

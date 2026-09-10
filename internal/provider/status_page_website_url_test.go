package provider

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestWebsiteURLValidator(t *testing.T) {
	ok := []string{
		"https://acme.example",
		"http://acme.example/status",
		"HTTPS://acme.example",
		"https://пример.укр/статус",
	}
	bad := []string{
		"acme.example",
		"javascript:alert(1)",
		"https://",
		"https:///status",
		"https://acme.example/" + string(make([]rune, 200)),
	}
	for _, in := range ok {
		resp := &validator.StringResponse{}
		websiteURLValidator{}.ValidateString(context.Background(), validator.StringRequest{
			Path: path.Root("website_url"), ConfigValue: types.StringValue(in),
		}, resp)
		if resp.Diagnostics.HasError() {
			t.Errorf("%q rejected: %v", in, resp.Diagnostics)
		}
	}
	for _, in := range bad {
		resp := &validator.StringResponse{}
		websiteURLValidator{}.ValidateString(context.Background(), validator.StringRequest{
			Path: path.Root("website_url"), ConfigValue: types.StringValue(in),
		}, resp)
		if !resp.Diagnostics.HasError() {
			t.Errorf("%q accepted", in)
		}
	}
}

package provider

import (
	"context"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

const testAccChannelSlack = `
resource "uptimepage_notification_channel" "c" {
  name = "acc-slack"
  config = {
    type = "slack"
    slack = { webhook_url = "https://hooks.slack.com/services/T/B/secret" }
  }
}
`

// TestAccChannelResource_basic: create, confirm empty re-plan, then import
// (write-only secrets ignored, since the API returns them redacted).
func TestAccChannelResource_basic(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6,
		Steps: []resource.TestStep{
			{
				Config: testAccChannelSlack,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("uptimepage_notification_channel.c", "id"),
					resource.TestCheckResourceAttr("uptimepage_notification_channel.c", "kind", "slack"),
				),
			},
			{
				ResourceName:            "uptimepage_notification_channel.c",
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"config.slack.webhook_url"},
			},
		},
	})
}

func TestMattermostHookPath(t *testing.T) {
	for url, want := range map[string]bool{
		"https://mm.example.com/hooks/abc123":        true,
		"https://mm.example.com/hooks/abc123/":       true,
		"https://co.test/mm/hooks/abc123":            true,
		"https://mm.example.com/hooks/abc?a=/b":      true,
		"https://mm.example.com/hooks/":              false,
		"https://mm.example.com/hooks/a/b":           false,
		"https://mm.example.com/api/v4/posts":        false,
		"https://mm.example.com/api/v4/x?u=/hooks/k": false,
	} {
		if got := mattermostHookPath.MatchString(url); got != want {
			t.Errorf("%s: got %v, want %v", url, got, want)
		}
	}
}

func TestMattermostURLValidator_NeverEchoesTheKey(t *testing.T) {
	const secret = "https://mm.example.com/api/v4/posts?k=zzSECRETKEYzz"
	var resp validator.StringResponse
	mattermostURLValidator{}.ValidateString(
		context.Background(),
		validator.StringRequest{
			Path:        path.Root("config").AtName("mattermost").AtName("webhook_url"),
			ConfigValue: types.StringValue(secret),
		},
		&resp,
	)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected a rejection")
	}
	for _, d := range resp.Diagnostics.Errors() {
		if strings.Contains(d.Detail()+d.Summary(), "zzSECRETKEYzz") {
			t.Errorf("diagnostic leaks the webhook key: %s", d.Detail())
		}
	}
}

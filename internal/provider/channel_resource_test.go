package provider

import (
	"testing"

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

package provider

import (
	"os"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

// testAccProtoV6 wires the in-process provider for the acceptance harness.
var testAccProtoV6 = map[string]func() (tfprotov6.ProviderServer, error){
	"uptimepage": providerserver.NewProtocol6WithError(New("test")()),
}

// testAccPreCheck skips when no real API token is configured. The harness also
// requires TF_ACC=1 and a terraform binary on PATH; otherwise resource.Test
// skips on its own.
func testAccPreCheck(t *testing.T) {
	if os.Getenv("UPTIMEPAGE_TOKEN") == "" {
		t.Skip("UPTIMEPAGE_TOKEN not set; skipping acceptance test")
	}
}

const testAccTargetBasic = `
resource "uptimepage_target" "t" {
  name     = "acc-http"
  interval = 60
  tags     = ["acc"]
  check = {
    type   = "http"
    url    = "https://example.com/healthz"
    method = "GET"
    expected_status = { kind = "exact", exact = 200 }
  }
}
`

// TestAccTargetResource_basic is the make-or-break check: create, then confirm
// the immediate re-plan is empty (no perpetual diff), then round-trip import.
func TestAccTargetResource_basic(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6,
		Steps: []resource.TestStep{
			{
				Config: testAccTargetBasic,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("uptimepage_target.t", "id"),
					resource.TestCheckResourceAttr("uptimepage_target.t", "interval", "60"),
					resource.TestCheckResourceAttr("uptimepage_target.t", "check.url", "https://example.com/healthz"),
				),
			},
			{
				ResourceName:            "uptimepage_target.t",
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"check.basic_auth", "check.bearer_token"},
			},
		},
	})
}

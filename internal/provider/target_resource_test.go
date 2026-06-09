package provider

import (
	"fmt"
	"os"
	"strconv"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
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
    type = "http"
    http = {
      url    = "https://example.com/healthz"
      method = "GET"
      expected_status = { kind = "exact", exact = 200 }
    }
  }
}
`

// testAccRegion is the region id the explicit-regions acceptance case sets. It
// must name an enabled region on the target test instance; override via
// UPTIMEPAGE_TEST_REGION when the instance's default region is named otherwise.
func testAccRegion() string {
	if r := os.Getenv("UPTIMEPAGE_TEST_REGION"); r != "" {
		return r
	}
	return "default"
}

// testAccTargetRegions defines two targets: "explicit" pins a single region,
// "computed" omits regions and must inherit the server-assigned set with no
// perpetual diff. %[1]s is the explicit region id, %[2]d the check interval.
const testAccTargetRegions = `
resource "uptimepage_target" "explicit" {
  name     = "acc-regions-explicit"
  interval = %[2]d
  regions  = [%[1]q]
  check = {
    type = "http"
    http = {
      url             = "https://example.com/healthz"
      expected_status = { kind = "exact", exact = 200 }
    }
  }
}

resource "uptimepage_target" "computed" {
  name     = "acc-regions-computed"
  interval = %[2]d
  check = {
    type = "http"
    http = {
      url             = "https://example.com/healthz"
      expected_status = { kind = "exact", exact = 200 }
    }
  }
}
`

// atLeastOneRegion asserts a set attribute (e.g. "regions.#") holds >= 1 element.
func atLeastOneRegion(name, attr string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[name]
		if !ok {
			return fmt.Errorf("resource %s not in state", name)
		}
		n, err := strconv.Atoi(rs.Primary.Attributes[attr])
		if err != nil {
			return fmt.Errorf("%s.%s = %q, not an int: %w", name, attr, rs.Primary.Attributes[attr], err)
		}
		if n < 1 {
			return fmt.Errorf("%s.%s = %d, want >= 1 (server auto-assigns regions)", name, attr, n)
		}
		return nil
	}
}

// TestAccTargetResource_regions covers both region modes: an explicitly pinned
// set is enforced and round-trips through import, while an omitted set is
// populated from the server's auto-assignment. The second step changes only the
// interval — resource.Test fails on a non-empty post-apply plan, so this proves
// that an unrelated update does not churn `regions` (the no-spurious-diff
// guarantee for the Optional+Computed set).
func TestAccTargetResource_regions(t *testing.T) {
	region := testAccRegion()
	cfg := fmt.Sprintf(testAccTargetRegions, region, 60)
	cfgUpdated := fmt.Sprintf(testAccTargetRegions, region, 120)
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6,
		Steps: []resource.TestStep{
			{
				Config: cfg,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("uptimepage_target.explicit", "regions.#", "1"),
					resource.TestCheckTypeSetElemAttr("uptimepage_target.explicit", "regions.*", region),
					// Omitted: server auto-assigns at least one region.
					atLeastOneRegion("uptimepage_target.computed", "regions.#"),
				),
			},
			{
				// Unrelated change; regions must stay put on both targets.
				Config: cfgUpdated,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("uptimepage_target.explicit", "interval", "120"),
					resource.TestCheckResourceAttr("uptimepage_target.explicit", "regions.#", "1"),
					resource.TestCheckTypeSetElemAttr("uptimepage_target.explicit", "regions.*", region),
					atLeastOneRegion("uptimepage_target.computed", "regions.#"),
				),
			},
			{
				ResourceName:            "uptimepage_target.explicit",
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"check.basic_auth", "check.bearer_token"},
			},
		},
	})
}

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
					resource.TestCheckResourceAttr("uptimepage_target.t", "check.http.url", "https://example.com/healthz"),
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

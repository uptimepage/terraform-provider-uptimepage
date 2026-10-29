package provider

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

// A group alerting as one, a monitor joining it by id, the group shown on a
// page as a section, and a component that adopts the member's row. %[1]t is
// alert_as_one, %[2]q the section heading.
const testAccMonitorGroupSection = `
resource "uptimepage_monitor_group" "g" {
  name         = "acc-group"
  alert_as_one = %[1]t
  tags         = ["acc"]
}

resource "uptimepage_target" "m" {
  name     = "acc-group-member"
  interval = 60
  tags     = ["own"]
  group_id = uptimepage_monitor_group.g.id
  check    = { type = "manual" }
}

resource "uptimepage_status_page" "p" {
  slug = "acc-group-page"
  name = "acc group page"
}

resource "uptimepage_status_page_section" "s" {
  status_page_id = uptimepage_status_page.p.id
  group_id       = uptimepage_monitor_group.g.id
  heading        = %[2]q
}

resource "uptimepage_status_page_component" "c" {
  status_page_id = uptimepage_status_page.p.id
  target_id      = uptimepage_target.m.id
  public_name    = "Member"

  depends_on = [uptimepage_status_page_section.s]
}

data "uptimepage_monitor_group" "byname" {
  name = uptimepage_monitor_group.g.name

  depends_on = [uptimepage_target.m]
}
`

func sectionImportID(s *terraform.State) (string, error) {
	rs, ok := s.RootModule().Resources["uptimepage_status_page_section.s"]
	if !ok {
		return "", fmt.Errorf("section not in state")
	}
	return rs.Primary.Attributes["status_page_id"] + ":" + rs.Primary.ID, nil
}

// TestAccMonitorGroup_sectionRoundTrip: every step's re-plan must be empty, so
// the member's own tags, the adopted row and the section heading all read back
// as written. Destroy then resets the adopted row instead of failing on it.
func TestAccMonitorGroup_sectionRoundTrip(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6,
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(testAccMonitorGroupSection, true, "Acc"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrPair("uptimepage_target.m", "group_id", "uptimepage_monitor_group.g", "id"),
					resource.TestCheckNoResourceAttr("uptimepage_target.m", "group_name"),
					resource.TestCheckResourceAttrPair("uptimepage_status_page_component.c", "section_id", "uptimepage_status_page_section.s", "id"),
					resource.TestCheckResourceAttr("uptimepage_status_page_component.c", "public_name", "Member"),
					resource.TestCheckResourceAttrPair("data.uptimepage_monitor_group.byname", "id", "uptimepage_monitor_group.g", "id"),
					resource.TestCheckResourceAttr("data.uptimepage_monitor_group.byname", "member_count", "1"),
				),
			},
			{
				ResourceName:      "uptimepage_monitor_group.g",
				ImportState:       true,
				ImportStateVerify: true,
			},
			{
				ResourceName:      "uptimepage_status_page_section.s",
				ImportState:       true,
				ImportStateIdFunc: sectionImportID,
				ImportStateVerify: true,
			},
			{
				Config: fmt.Sprintf(testAccMonitorGroupSection, false, "Acc DNS"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("uptimepage_monitor_group.g", "alert_as_one", "false"),
					resource.TestCheckResourceAttr("uptimepage_status_page_section.s", "heading", "Acc DNS"),
				),
			},
		},
	})
}

// A label group made by an untrimmed group_name, then the same monitor moved
// to a declared group by id: the first apply must keep the config's spelling,
// and the switch must send group_id alone.
const testAccTargetByName = `
resource "uptimepage_target" "m" {
  name       = "acc-group-by-name"
  interval   = 60
  group_name = " acc label "
  check      = { type = "manual" }
}
`

const testAccTargetByID = `
resource "uptimepage_monitor_group" "g" {
  name = "acc label 2"
}

resource "uptimepage_target" "m" {
  name     = "acc-group-by-name"
  interval = 60
  group_id = uptimepage_monitor_group.g.id
  check    = { type = "manual" }
}
`

func TestAccTarget_groupNameThenGroupID(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6,
		Steps: []resource.TestStep{
			{
				Config: testAccTargetByName,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("uptimepage_target.m", "group_name", " acc label "),
					resource.TestCheckNoResourceAttr("uptimepage_target.m", "group_id"),
				),
			},
			{
				Config: testAccTargetByID,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrPair("uptimepage_target.m", "group_id", "uptimepage_monitor_group.g", "id"),
					resource.TestCheckNoResourceAttr("uptimepage_target.m", "group_name"),
				),
			},
		},
	})
}

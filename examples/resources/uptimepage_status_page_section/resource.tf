# uptimepage_monitor_group.dns and uptimepage_target.ns as in the
# uptimepage_monitor_group example.

resource "uptimepage_status_page" "public" {
  slug = "acme"
  name = "Acme Status"
}

# Show the group as a live section: its members follow the group onto and off
# the page, under one heading with their combined state.
resource "uptimepage_status_page_section" "dns" {
  status_page_id = uptimepage_status_page.public.id
  group_id       = uptimepage_monitor_group.dns.id
  heading        = "DNS"
}

# A member's own public name and order. The section already put the monitor
# on the page, so this adopts its row.
resource "uptimepage_status_page_component" "ns" {
  status_page_id = uptimepage_status_page.public.id
  target_id      = uptimepage_target.ns.id
  public_name    = "Nameservers"

  depends_on = [uptimepage_status_page_section.dns]
}

# A group made in the app, or by a monitor's group_name.
data "uptimepage_monitor_group" "dns" {
  name = "example.com DNS"
}

resource "uptimepage_target" "mx" {
  name     = "example.com MX"
  interval = 300
  group_id = data.uptimepage_monitor_group.dns.id
  check = {
    type = "dns"
    dns = {
      domain      = "example.com"
      record_type = "MX"
    }
  }
}

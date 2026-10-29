resource "uptimepage_notification_channel" "ops" {
  name = "ops slack"
  config = {
    type  = "slack"
    slack = { webhook_url = var.slack_webhook_url }
  }
}

# One incident and one page for every DNS record of the domain: the first
# record down opens the group incident, later ones join it.
resource "uptimepage_monitor_group" "dns" {
  name                   = "example.com DNS"
  alert_as_one           = true
  alerts                 = [{ channel_id = uptimepage_notification_channel.ops.id }]
  renotify_interval_secs = 1800
  alert_again            = true
  tags                   = ["dns", "p0"]
}

resource "uptimepage_target" "ns" {
  name     = "example.com NS"
  interval = 300
  group_id = uptimepage_monitor_group.dns.id
  check = {
    type = "dns"
    dns = {
      domain      = "example.com"
      record_type = "NS"
    }
  }
}

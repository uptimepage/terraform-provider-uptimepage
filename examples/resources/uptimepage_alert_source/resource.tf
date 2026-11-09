resource "uptimepage_notification_channel" "ops" {
  name = "ops slack"
  config = {
    type  = "slack"
    slack = { webhook_url = var.slack_webhook_url }
  }
}

# Alertmanager posts its alert groups here; one group is one incident, and
# its recovery closes it. Give each team's receiver a source of its own.
resource "uptimepage_alert_source" "prometheus" {
  name   = "prod alertmanager"
  kind   = "alertmanager"
  alerts = [{ channel_id = uptimepage_notification_channel.ops.id }]
}

# A Grafana contact point that signs what it sends. The secret is write-only
# (Terraform 1.11+): bump the version to rotate it.
resource "uptimepage_alert_source" "grafana" {
  name                      = "prod grafana"
  kind                      = "grafana"
  escalation_policy_id      = var.oncall_policy_id
  signing_secret_wo         = var.grafana_signing_secret
  signing_secret_wo_version = 1
}

# Hand the URL to the tool's own configuration; it is the source's credential.
output "alertmanager_webhook_url" {
  value     = uptimepage_alert_source.prometheus.ingest_url
  sensitive = true
}

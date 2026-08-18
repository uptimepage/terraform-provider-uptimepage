# A heartbeat monitor and the URL its job reports to. The URL is minted by the
# API when the target is created, so it is read back rather than configured.
resource "uptimepage_target" "nightly_backup" {
  name = "nightly backup"
  # How often the deadline is checked, not how often the job runs. It has to
  # fit inside period_ms + grace_ms.
  interval = 300

  check = {
    type = "heartbeat"
    heartbeat = {
      period_ms = 86400000 # the job runs daily
      grace_ms  = 3600000  # an hour late is still fine
      # A run that opens with /start and then hangs for four hours has failed,
      # even though the daily deadline has not passed yet.
      max_runtime_ms = 14400000
    }
  }
}

data "uptimepage_heartbeat" "nightly_backup" {
  target_id = uptimepage_target.nightly_backup.id
}

# Anyone holding this URL can report the job healthy or failed. Feed it to the
# job through a secret store rather than printing it.
output "backup_ping_url" {
  value     = data.uptimepage_heartbeat.nightly_backup.ping_url
  sensitive = true
}

resource "uptimepage_notification_channel" "slack" {
  name = "ops slack"
  config = {
    type = "slack"
    slack = {
      webhook_url = var.slack_webhook_url
    }
  }
}

resource "uptimepage_notification_channel" "webhook" {
  name = "ops webhook"
  config = {
    type = "webhook"
    webhook = {
      url = "https://hooks.example.com/uptimepage"
      headers = {
        "Authorization" = "Bearer ${var.webhook_token}"
      }
    }
  }
}

resource "uptimepage_notification_channel" "telegram" {
  name = "ops telegram"
  config = {
    type = "telegram"
    telegram = {
      bot_token = var.telegram_bot_token
      chat_id   = "-1001234567890"
    }
  }
}

# Reference a channel from a target's alert binding.
resource "uptimepage_target" "api" {
  name     = "api"
  interval = 60
  check = {
    type = "http"
    http = {
      url             = "https://example.com/healthz"
      expected_status = { kind = "exact", exact = 200 }
    }
  }
  alerts = [{
    channel_id     = uptimepage_notification_channel.slack.id
    after_failures = 3
  }]
}

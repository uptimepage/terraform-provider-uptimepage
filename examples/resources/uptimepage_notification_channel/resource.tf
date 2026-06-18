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

resource "uptimepage_notification_channel" "discord" {
  name = "ops discord"
  config = {
    type = "discord"
    discord = {
      webhook_url = var.discord_webhook_url
    }
  }
}

resource "uptimepage_notification_channel" "email" {
  name = "ops mail"
  config = {
    type = "email"
    email = {
      to = "oncall@example.com"
    }
  }
  # Email channels deliver only after the recipient confirms the
  # verification mail; verified_at flips from null once they do.
}

# Bring-your-own SMS gateway. Set provider and that gateway's credentials.
resource "uptimepage_notification_channel" "sms_twilio" {
  name = "oncall sms"
  config = {
    type = "sms"
    sms = {
      provider    = "twilio"
      to          = "+15551234567"
      from        = "+15557654321"
      account_sid = var.twilio_account_sid
      auth_token  = var.twilio_auth_token
    }
  }
}

# Sinch is region-routed: region selects the API cluster your account lives in.
resource "uptimepage_notification_channel" "sms_sinch" {
  name = "oncall sms eu"
  config = {
    type = "sms"
    sms = {
      provider        = "sinch"
      to              = "+15551234567"
      from            = "Acme"
      service_plan_id = var.sinch_service_plan_id
      api_token       = var.sinch_api_token
      region          = "eu"
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

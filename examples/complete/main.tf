terraform {
  required_version = ">= 1.0"

  required_providers {
    uptimepage = {
      source  = "uptimepage/uptimepage"
      version = "~> 0.5"
    }
  }
}

variable "uptimepage_token" {
  description = "Uptimepage API token with targets:write, channels:write, and status_page:write scopes."
  type        = string
  sensitive   = true
}

variable "uptimepage_org" {
  description = "Organization slug that owns the monitor and status page."
  type        = string
}

variable "monitored_url" {
  description = "Public health endpoint to monitor."
  type        = string
}

variable "slack_webhook_url" {
  description = "Slack incoming-webhook URL for uptime alerts."
  type        = string
  sensitive   = true
}

variable "status_page_slug" {
  description = "Public status-page subdomain slug."
  type        = string
  default     = "acme-status"
}

provider "uptimepage" {
  # The endpoint defaults to https://app.uptimepage.dev.
  token = var.uptimepage_token
  org   = var.uptimepage_org
}

resource "uptimepage_notification_channel" "slack" {
  name = "production uptime"
  config = {
    type = "slack"
    slack = {
      webhook_url = var.slack_webhook_url
    }
  }
}

resource "uptimepage_target" "api" {
  name     = "production API"
  interval = 60

  check = {
    type = "http"
    http = {
      url             = var.monitored_url
      expected_status = { kind = "exact", exact = 200 }
    }
  }

  alerts = [{
    channel_id     = uptimepage_notification_channel.slack.id
    after_failures = 3
  }]
}

resource "uptimepage_status_page" "public" {
  slug    = var.status_page_slug
  name    = "Acme Status"
  enabled = true

  display_name    = "Acme Status"
  about           = "Live status of Acme's public services."
  brand_color     = "#0a7cff"
  style           = "default"
  show_powered_by = true
}

resource "uptimepage_status_page_component" "api" {
  status_page_id = uptimepage_status_page.public.id
  target_id      = uptimepage_target.api.id

  public_name  = "API"
  public_group = "Core services"
  sort_order   = 0
}

output "status_page_url" {
  description = "Customer-facing status page created by this configuration."
  value       = uptimepage_status_page.public.status_url
}

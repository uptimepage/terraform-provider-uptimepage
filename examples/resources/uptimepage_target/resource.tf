resource "uptimepage_target" "api" {
  name     = "api prod"
  interval = 60
  tags     = ["prod", "api"]

  check = {
    type = "http"
    http = {
      url    = "https://example.com/healthz"
      method = "GET"

      expected_status = {
        kind  = "exact"
        exact = 200
      }

      headers = {
        "X-Health-Check" = "uptimepage"
      }
    }
  }
}

# HTTP range matcher with basic auth. password_wo (Terraform 1.11+) never
# reaches state; bump password_wo_version to rotate it. On older Terraform use
# password instead, which persists to state.
resource "uptimepage_target" "admin" {
  name     = "admin panel"
  interval = 120

  check = {
    type = "http"
    http = {
      url = "https://admin.example.com/"

      expected_status = {
        kind = "range"
        range = {
          min = 200
          max = 299
        }
      }

      basic_auth = {
        username            = var.admin_user
        password_wo         = var.admin_password
        password_wo_version = 1
      }
    }
  }
}

# TCP connect check.
resource "uptimepage_target" "db" {
  name     = "postgres"
  interval = 60

  check = {
    type = "tcp"
    tcp = {
      host = "db.example.com"
      port = 5432
    }
  }
}

# Inbound dead-man's-switch. Nothing is sent to this monitor: the job reports
# in, and silence past period_ms + grace_ms opens an incident. Read the URL the
# job posts to with the uptimepage_heartbeat data source.
resource "uptimepage_target" "nightly_backup" {
  name = "nightly backup"
  # How often the deadline is checked, not how often the job runs.
  interval = 300

  check = {
    type = "heartbeat"
    heartbeat = {
      period_ms = 86400000
      grace_ms  = 3600000
    }
  }
}

# ICMP echo check. Reaches a host that answers no TCP port at all, such as a
# bare VM or an appliance. The host has to be publicly routable: the SSRF guard
# refuses loopback, private and link-local addresses unless the instance sets
# security.allow_private_targets, so this cannot reach a LAN gateway on the
# hosted service.
resource "uptimepage_target" "edge_router" {
  name     = "edge router"
  interval = 60

  check = {
    type = "ping"
    ping = {
      host = "edge.example.com"
    }
  }
}

# TLS certificate expiry check. A certificate changes on renewal, so twice a
# day is plenty; the alert fires warn_days ahead either way.
resource "uptimepage_target" "cert" {
  name     = "cert example.com"
  interval = 43200

  check = {
    type = "tls_cert"
    tls_cert = {
      host          = "example.com"
      port          = 443
      warn_days     = 30
      critical_days = 7
    }
  }
}

# Domain registration expiry check. Registry data changes about once a year and
# RDAP rate-limits by source address, so keep this daily, especially when a
# loop creates one per domain.
resource "uptimepage_target" "domain" {
  name     = "domain example.com"
  interval = 86400

  check = {
    type = "domain_expiry"
    domain_expiry = {
      domain        = "example.com"
      warn_days     = 30
      critical_days = 7
    }
  }
}

# DNS resolution check.
resource "uptimepage_target" "dns" {
  name     = "dns api.example.com"
  interval = 300

  check = {
    type = "dns"
    dns = {
      domain            = "api.example.com"
      record_type       = "A"
      expected_contains = "192.0.2"
    }
  }
}

# Browser login flow check. Runs a headless browser through the steps and
# asserts the result. Put credentials in an org secret and reference them as
# {{name}}; an inline literal would be stored as typed. Far heavier than a
# single probe, so it runs less often.
resource "uptimepage_target" "login" {
  name     = "app login"
  interval = 900

  check = {
    type = "flow"
    flow = {
      start_url = "https://app.example.com/login"
      steps = [
        { op = "fill", selector = "#username", value = "monitor@example.com" },
        { op = "fill", selector = "#password", value = "{{login_password}}" },
        { op = "click", selector = "button[type=submit]" },
        { op = "assert_url", contains = "/dashboard" },
        { op = "assert_text", contains = "Signed in" },
      ]
    }
  }
}

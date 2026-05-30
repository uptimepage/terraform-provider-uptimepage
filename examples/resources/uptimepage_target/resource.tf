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

# HTTP range matcher with basic auth (write-only secret).
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
        username = var.admin_user
        password = var.admin_password
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

# TLS certificate expiry check.
resource "uptimepage_target" "cert" {
  name     = "cert example.com"
  interval = 3600

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

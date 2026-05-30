resource "uptimepage_target" "api" {
  name     = "api prod"
  interval = 60
  tags     = ["prod", "api"]

  check = {
    type   = "http"
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

# Range matcher with basic auth (write-only secret).
resource "uptimepage_target" "admin" {
  name     = "admin panel"
  interval = 120

  check = {
    type = "http"
    url  = "https://admin.example.com/"

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

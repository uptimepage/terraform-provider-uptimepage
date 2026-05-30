terraform {
  required_providers {
    uptimepage = {
      source = "uptimepage/uptimepage"
    }
  }
}

provider "uptimepage" {
  # endpoint defaults to https://uptimepage.dev; point it at your app host
  # (e.g. https://app.uptimepage.dev) for a hosted/self-managed instance.
  endpoint = "https://app.uptimepage.dev"

  # token may also be supplied via UPTIMEPAGE_TOKEN
  token = var.uptimepage_token

  # org scopes API-token requests to one organization (slug). Required for
  # managing resources with a token. May also be set via UPTIMEPAGE_ORG.
  org = "your-org-slug"
}

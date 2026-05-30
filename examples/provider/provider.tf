terraform {
  required_providers {
    uptimepage = {
      source = "uptimepage/uptimepage"
    }
  }
}

provider "uptimepage" {
  # endpoint defaults to https://uptimepage.dev
  # token may also be supplied via UPTIMEPAGE_TOKEN
  token = var.uptimepage_token
}

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
}

resource "uptimepage_status_page" "public" {
  slug = "acme"
  name = "Acme Status"
}

# Curate the monitor onto the page, with per-page overrides.
resource "uptimepage_status_page_component" "api" {
  status_page_id = uptimepage_status_page.public.id
  target_id      = uptimepage_target.api.id

  public_name  = "API"
  public_group = "Core services"
  sort_order   = 0
}

resource "uptimepage_status_page" "public" {
  slug    = "acme"
  name    = "Acme Status"
  enabled = true

  display_name    = "Acme Status"
  about           = "Live status of Acme's public services."
  brand_color     = "#0a7cff"
  style           = "default"
  show_powered_by = true
}

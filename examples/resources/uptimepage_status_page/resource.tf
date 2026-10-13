resource "uptimepage_status_page" "public" {
  slug    = "acme"
  name    = "Acme Status"
  enabled = true

  display_name    = "Acme Status"
  about           = "Live status of Acme's public services."
  brand_color     = "#0a7cff"
  style           = "default"
  show_powered_by = true

  website_url      = "https://acme.example"
  hide_from_search = false
}

# Only the people on the list can open this one: anyone at the partner's
# domain, and one outside contractor.
resource "uptimepage_status_page" "partners" {
  slug    = "acme-partners"
  name    = "Acme Partner Status"
  enabled = true

  access = {
    protected = true
    email = {
      session_days = 30
    }
    rules = ["partner.example", "k.lindner@example.org"]
  }
}

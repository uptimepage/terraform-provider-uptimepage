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
    protected    = true
    session_days = 30
    rules        = ["partner.example", "k.lindner@example.org"]
  }
}

# One shared password instead of a list. It never reaches plan or state, so
# Terraform cannot show it later: keep it where the people who need it get it
# from, and bump value_wo_version to set a new one.
variable "dealer_password" {
  type      = string
  ephemeral = true
  sensitive = true
}

resource "uptimepage_status_page" "dealer" {
  slug    = "acme-dealer"
  name    = "Acme Dealer Status"
  enabled = true

  access = {
    protected = true
    email     = { enabled = false }
    password = {
      enabled          = true
      value_wo         = var.dealer_password
      value_wo_version = 1
    }
  }
}

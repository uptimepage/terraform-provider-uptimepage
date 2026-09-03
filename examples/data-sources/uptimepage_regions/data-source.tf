# The probe regions this instance serves. Reading them beats hard-coding slugs:
# a self-hosted fleet has its own, and the hosted one opens new ones over time.
data "uptimepage_regions" "all" {}

# Probe from everywhere the fleet can reach. Your plan still caps how many
# regions one monitor may hold, so this fails the apply on a fleet larger than
# the cap rather than quietly trimming the set.
resource "uptimepage_target" "checkout" {
  name     = "checkout"
  interval = 60
  regions  = data.uptimepage_regions.all.ids

  check = {
    type = "http"
    http = {
      url             = "https://example.com/healthz"
      expected_status = { kind = "exact", exact = 200 }
    }
  }
}

# Or pick by geography, without naming ids the next region rollout would date.
resource "uptimepage_target" "eu_only" {
  name     = "eu checkout"
  interval = 60
  regions = [
    for r in data.uptimepage_regions.all.regions : r.id if r.continent == "europe"
  ]

  check = {
    type = "http"
    http = {
      url             = "https://eu.example.com/healthz"
      expected_status = { kind = "exact", exact = 200 }
    }
  }
}

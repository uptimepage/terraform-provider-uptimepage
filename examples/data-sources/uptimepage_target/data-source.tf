data "uptimepage_target" "existing" {
  id = "01h7m8z4n6v0e1m7v7y6x8x8x8"
}

output "target_name" {
  value = data.uptimepage_target.existing.name
}

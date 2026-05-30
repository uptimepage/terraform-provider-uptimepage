# Import an existing target by its UUID.
# Write-only secrets (check.basic_auth, check.bearer_token) cannot be read back
# from the API, so set them in config after import.
terraform import uptimepage_target.api 01h7m8z4n6v0e1m7v7y6x8x8x8

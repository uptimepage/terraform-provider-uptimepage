# Import an existing notification channel by its UUID.
# Write-only config secrets (url, headers, webhook_url, bot_token) cannot be
# read back from the API, so set them in config after import.
terraform import uptimepage_notification_channel.slack 01h7m8z4n6v0e1m7v7y6x8x8x8

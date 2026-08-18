# Import an existing notification channel by its UUID.
# Write-only config secrets (url, headers, webhook_url, bot_token) cannot be
# read back from the API, so set them in config after import.
terraform import uptimepage_notification_channel.slack 0192aaaa-bbbb-cccc-dddd-eeeeeeeeeeee

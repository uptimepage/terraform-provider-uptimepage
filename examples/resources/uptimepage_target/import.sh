# Import an existing target by its UUID.
# Secrets (check.basic_auth, check.bearer_token) cannot be read back from the
# API, so set them in config after import.
terraform import uptimepage_target.api 0192f3a4-5b6c-7d8e-9f01-23456789abcd

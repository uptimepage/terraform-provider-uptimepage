# Import an existing source by its UUID, the last part of its page URL in the
# app. A signing secret never comes back from the API, so set it in config.
terraform import uptimepage_alert_source.prometheus 0192f3a4-5b6c-7d8e-9f01-23456789abcd

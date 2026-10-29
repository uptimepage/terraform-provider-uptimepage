# Import an existing group by its UUID, the last part of its settings page
# URL. A group made by a monitor's group_name or in the app already exists, so
# import it rather than declaring it fresh.
terraform import uptimepage_monitor_group.dns 0192f3a4-5b6c-7d8e-9f01-23456789abcd

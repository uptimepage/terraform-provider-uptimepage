# terraform-provider-uptimepage

Terraform provider for [UptimePage](https://uptimepage.dev) — manage your monitors and notification channels as code against the `/api/v1` REST API.

## Usage

```hcl
terraform {
  required_providers {
    uptimepage = {
      source = "uptimepage/uptimepage"
    }
  }
}

provider "uptimepage" {
  # endpoint defaults to https://app.uptimepage.dev (the hosted service).
  # Set it for a self-hosted instance, e.g. https://uptime.example.com.
  token = var.uptimepage_token # or set UPTIMEPAGE_TOKEN
  org   = "your-org-slug"      # required for managed resources; or UPTIMEPAGE_ORG
}

resource "uptimepage_notification_channel" "slack" {
  name = "ops slack"
  config = {
    type  = "slack"
    slack = { webhook_url = var.slack_webhook_url }
  }
}

resource "uptimepage_target" "api" {
  name     = "api prod"
  interval = 60
  check = {
    type = "http"
    http = {
      url             = "https://example.com/healthz"
      expected_status = { kind = "exact", exact = 200 }
    }
  }
  alerts = [{
    channel_id     = uptimepage_notification_channel.slack.id
    after_failures = 3
  }]
}
```

## Authentication

The provider authenticates with an API token (`Authorization: Bearer sm_live_…`), created from the UptimePage **API tokens** page (requires a verified email). Supply it via the `token` provider attribute or the `UPTIMEPAGE_TOKEN` environment variable.

Grant the token the **least scope** the provider needs: `targets:write` + `channels:write` covers both managed resources (`write` implies `read`, and the provider only deletes during `terraform destroy`). Add `targets:delete` + `channels:delete` only if you run `destroy`. For defence in depth, **bind the token to the org** you manage so a leaked token can't reach your other orgs — a bound token then requires `org` to match it (else `403 ORG_HEADER_MISMATCH`).

API tokens are user-scoped, so every managed-resource request must also name an organization — set `org` (the org **slug**) on the provider, or the `UPTIMEPAGE_ORG` environment variable. It is sent as the `X-Uptimepage-Org` header; without it the API returns `400 ORG_REQUIRED`. Find your slug at `GET /api/v1/orgs` or in the dashboard URL.

## Resources & data sources

| Name | Kind | Notes |
|------|------|-------|
| `uptimepage_target` | resource | Monitors. Check types: `http`, `tcp`, `tls_cert`, `domain_expiry`, `dns`. |
| `uptimepage_notification_channel` | resource | `webhook`, `slack`, `telegram`. |
| `uptimepage_target` | data source | Look up a target by id. |

Full reference under [`docs/`](docs/), generated from the schema.

## Managed-by badge

The provider identifies itself on every request (a `terraform-provider-uptimepage` User-Agent), so UptimePage knows which resources Terraform manages. Those monitors and channels show a `terraform` chip in the web UI, with a banner on the monitor detail page.

It's informational — the UI doesn't lock the resource. But editing a managed resource in the UI flips its badge and **the change is overwritten on the next `terraform apply`**, since your configuration stays the source of truth. Make changes in Terraform, not the UI.

## Write-only secrets

Some fields are write-only: the API returns them redacted (`***`) on read, so the provider keeps the value from your configuration/state and **cannot detect out-of-band changes** to them. Rotating such a secret means changing it in your configuration. Affected fields:

- `uptimepage_target` → `check.http.basic_auth`, `check.http.bearer_token`
- `uptimepage_notification_channel` → `config.webhook.url`, `config.webhook.headers`, `config.slack.webhook_url`, `config.telegram.bot_token`

On `terraform import`, these land empty — set them in configuration afterwards.

## Development

```sh
make check   # gofmt + vet + build + unit tests
```

Requires Go 1.26+.

Regenerate docs after a schema change:

```sh
go run github.com/hashicorp/terraform-plugin-docs/cmd/tfplugindocs generate --provider-name uptimepage
```

Acceptance tests hit a real API and are gated on a token:

```sh
TF_ACC=1 UPTIMEPAGE_TOKEN=sm_live_… go test ./internal/provider -run TestAcc -timeout 20m
```

## Releasing

Tags matching `v*` trigger the `release` workflow, which builds signed archives with GoReleaser and publishes a GitHub release the Terraform Registry consumes. Requires the repository secrets `GPG_PRIVATE_KEY` and `PASSPHRASE`, and the corresponding public key registered with the Terraform Registry.

## Compatibility

| Provider | Terraform | Protocol |
|----------|-----------|----------|
| 0.x | >= 1.0 | 6 |

## License

TBD.

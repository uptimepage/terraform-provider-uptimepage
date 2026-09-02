# terraform-provider-uptimepage

Terraform provider for [Uptimepage](https://uptimepage.dev/terraform-uptime-monitoring) — manage monitors, notification channels, and public status pages as code against the `/api/v1` REST API. The [Terraform setup guide](https://uptimepage.dev/docs/terraform) covers tokens, organization scope, imports, and the hosted-service workflow.

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
  regions  = ["us-east", "apac-sg"] # omit to probe from every region
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

For a copy-ready configuration that connects a monitor to Slack and publishes it on a customer-facing status page, see the [complete hosted example](examples/complete/main.tf).

## Authentication

The provider authenticates with an API token (`Authorization: Bearer sm_live_…`), created from the Uptimepage **API tokens** page (requires a verified email). Supply it via the `token` provider attribute or the `UPTIMEPAGE_TOKEN` environment variable.

Grant the token the **least scope** the provider needs: `targets:write` + `channels:write` covers monitors and channels, and `status_page:write` covers status pages and their components (`write` implies `read`, and the provider only deletes during `terraform destroy`). Add the matching `:delete` scopes only if you run `destroy`. Status-page writes are **owner-only**, so the token must belong to an org owner (a non-owner member's token gets `403` on page changes). For defence in depth, **bind the token to the org** you manage so a leaked token can't reach your other orgs — a bound token then requires `org` to match it (else `403 ORG_HEADER_MISMATCH`).

API tokens are user-scoped, so every managed-resource request must also name an organization — set `org` (the org **slug**) on the provider, or the `UPTIMEPAGE_ORG` environment variable. It is sent as the `X-Uptimepage-Org` header; without it the API returns `400 ORG_REQUIRED`. Find your slug at `GET /api/v1/orgs` or in the dashboard URL.

## Resources & data sources

| Name | Kind | Notes |
|------|------|-------|
| `uptimepage_target` | resource | Monitors. Check types: `http`, `tcp`, `ping`, `heartbeat`, `tls_cert`, `domain_expiry`, `dns`, `flow`. |
| `uptimepage_notification_channel` | resource | `webhook`, `slack`, `telegram`, `discord`, `msteams`, `google_chat`, `email`, `pagerduty`, `ntfy`, `pushover`, `whatsapp`, `sms`. |
| `uptimepage_status_page` | resource | A public status page: slug, branding, theme. Owner-only. |
| `uptimepage_status_page_component` | resource | Curates one monitor onto a page, with per-page overrides. |
| `uptimepage_target` | data source | Look up a target by id. |
| `uptimepage_heartbeat` | data source | The URL a heartbeat job reports to. Sensitive: holding it is enough to report the job healthy or failed. |

Full reference under [`docs/`](docs/), generated from the schema.

## Regions

`uptimepage_target.regions` selects which regions a monitor probes from, as operator-defined slugs (e.g. `us-east`, `apac-sg`). It is **optional + computed**:

- **Omit it** and the server auto-assigns every available region on create (up to your plan's cap). That set is read back into state, so there is no perpetual diff.
- **Set it** to pin an exact set; the set is replaced wholesale on each change. At least one region is required (an empty set is rejected at plan time), and unknown or disabled region ids surface the API's `REGION_INVALID` error.

Regions are managed through a target sub-resource (`/api/v1/targets/{id}/regions`); reading needs the `targets:read` scope and writing needs `targets:write` — both already covered by the `targets:write` scope the provider uses. There is currently no public endpoint that lists the full region catalog, so configs must name region ids directly.

## Managed-by badge

The provider identifies itself on every request (a `terraform-provider-uptimepage` User-Agent), so Uptimepage knows which resources Terraform manages. Those monitors and channels show a `terraform` chip in the web UI, with a banner on the monitor detail page.

It's informational — the UI doesn't lock the resource. But editing a managed resource in the UI flips its badge and **the change is overwritten on the next `terraform apply`**, since your configuration stays the source of truth. Make changes in Terraform, not the UI.

## Secrets

The API returns secret fields redacted (`***`) on read, so the provider keeps the value from your configuration/state and **cannot detect out-of-band changes** to them. Rotating such a secret means changing it in your configuration. Affected fields:

- `uptimepage_target` → `check.http.basic_auth`, `check.http.bearer_token`
- `uptimepage_notification_channel` → `config.webhook.url`, `config.webhook.headers`, `config.slack.webhook_url`, `config.telegram.bot_token`

On `terraform import`, these land empty — set them in configuration afterwards.

### Write-only arguments (Terraform 1.11+)

The target check secrets also come as [write-only arguments](https://developer.hashicorp.com/terraform/language/resources/ephemeral#write-only-arguments), which are sent to the API on apply but never persisted to state or plan:

- `check.http.basic_auth.password_wo` + `password_wo_version`
- `check.http.bearer_token_wo` + `bearer_token_wo_version`

Because the value never touches state, Terraform cannot diff it — bump the paired `*_wo_version` to rotate the secret. Forgetting the bump is silent: apply succeeds with no diff and the new value is not sent. The classic in-state attributes remain available for older Terraform; setting both variants of the same secret is a validation error.

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

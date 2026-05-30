# terraform-provider-uptimepage

Terraform provider for [UptimePage](https://uptimepage.dev) — manage your monitors, notification channels, and status pages as code against the `/api/v1` REST API.

> Status: early development. Phase 0 (API client transport) complete; resources land in subsequent phases.

## Layout

- `internal/client` — UptimePage API transport. Plain Go, no Terraform deps. Unit-tested against `httptest`.
- `internal/provider` — Terraform plugin-framework glue (provider, resources, data sources).

## Development

```sh
make check   # gofmt + vet + build + unit tests
make test
```

Requires Go 1.26+.

## Authentication

The provider authenticates with an API token (`Authorization: Bearer sm_live_…`), created from the UptimePage web UI (API tokens page; requires a verified email). Configure via the `token` provider attribute or the `UPTIMEPAGE_TOKEN` environment variable.

## License

TBD.

// Package provider wires the Terraform provider: config, auth (endpoint/token
// with env fallback and fail-fast), and the resources/data sources it serves.
package provider

import (
	"cmp"
	"context"
	"os"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/uptimepage/terraform-provider-uptimepage/internal/client"
)

const (
	envEndpoint = "UPTIMEPAGE_ENDPOINT"
	envToken    = "UPTIMEPAGE_TOKEN"
	envOrg      = "UPTIMEPAGE_ORG"
)

// Ensure the implementation satisfies the framework interface at compile time.
var _ provider.Provider = (*Provider)(nil)

type Provider struct {
	version string
}

// providerModel mirrors the provider config block.
type providerModel struct {
	Endpoint types.String `tfsdk:"endpoint"`
	Token    types.String `tfsdk:"token"`
	Org      types.String `tfsdk:"org"`
}

// New returns the factory the provider server expects.
func New(version string) func() provider.Provider {
	return func() provider.Provider {
		return &Provider{version: version}
	}
}

func (p *Provider) Metadata(_ context.Context, _ provider.MetadataRequest, resp *provider.MetadataResponse) {
	resp.TypeName = "uptimepage"
	resp.Version = p.version
}

func (p *Provider) Schema(_ context.Context, _ provider.SchemaRequest, resp *provider.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manage UptimePage monitors and notification channels as code.",
		Attributes: map[string]schema.Attribute{
			"endpoint": schema.StringAttribute{
				Optional:    true,
				Description: "Base URL of the UptimePage API. Defaults to " + client.DefaultEndpoint + ". May also be set via the " + envEndpoint + " environment variable.",
			},
			"token": schema.StringAttribute{
				Optional:    true,
				Sensitive:   true,
				Description: "API token (Bearer). Create one from the UptimePage API tokens page. May also be set via the " + envToken + " environment variable.",
			},
			"org": schema.StringAttribute{
				Optional:    true,
				Description: "Organization slug to scope API-token requests to (sent as the X-Uptimepage-Org header). Required for token auth against managed resources. May also be set via the " + envOrg + " environment variable.",
			},
		},
	}
}

func (p *Provider) Configure(ctx context.Context, req provider.ConfigureRequest, resp *provider.ConfigureResponse) {
	var cfg providerModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// A value referencing an unknown (another resource's not-yet-known output)
	// cannot be resolved at configure time — fail with a clear pointer.
	if cfg.Endpoint.IsUnknown() {
		resp.Diagnostics.AddAttributeError(path.Root("endpoint"),
			"Unknown API endpoint",
			"The endpoint cannot depend on an unknown value. Set it to a known value or via "+envEndpoint+".")
	}
	if cfg.Token.IsUnknown() {
		resp.Diagnostics.AddAttributeError(path.Root("token"),
			"Unknown API token",
			"The token cannot depend on an unknown value. Set it to a known value or via "+envToken+".")
	}
	if cfg.Org.IsUnknown() {
		resp.Diagnostics.AddAttributeError(path.Root("org"),
			"Unknown organization",
			"The org cannot depend on an unknown value. Set it to a known value or via "+envOrg+".")
	}
	if resp.Diagnostics.HasError() {
		return
	}

	endpoint, token := resolveSettings(
		cfg.Endpoint.ValueString(), cfg.Token.ValueString(),
		os.Getenv(envEndpoint), os.Getenv(envToken),
	)
	org := cmp.Or(cfg.Org.ValueString(), os.Getenv(envOrg))

	if token == "" {
		resp.Diagnostics.AddAttributeError(path.Root("token"),
			"Missing API token",
			"Set the provider `token` attribute or the "+envToken+" environment variable. "+
				"Create a token from the UptimePage API tokens page (requires a verified email).")
		return
	}

	c := client.New(endpoint, token, org, nil).WithUserAgentVersion(p.version)
	resp.ResourceData = c
	resp.DataSourceData = c
}

func (p *Provider) Resources(_ context.Context) []func() resource.Resource {
	return []func() resource.Resource{
		newTargetResource,
		newChannelResource,
		newStatusPageResource,
		newStatusPageComponentResource,
	}
}

func (p *Provider) DataSources(_ context.Context) []func() datasource.DataSource {
	return []func() datasource.DataSource{
		newTargetDataSource,
	}
}

// resolveSettings applies precedence config > env per setting. Pure (env values
// passed in) so it is unit-testable without touching the process environment.
// An unset endpoint is returned empty on purpose: client.New supplies
// DefaultEndpoint, keeping the default in exactly one place.
func resolveSettings(cfgEndpoint, cfgToken, envEndpointVal, envTokenVal string) (endpoint, token string) {
	return cmp.Or(cfgEndpoint, envEndpointVal), cmp.Or(cfgToken, envTokenVal)
}

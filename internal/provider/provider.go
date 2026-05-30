// Package provider is the Terraform glue: a minimal provider that builds and
// serves; schema, Configure, and resources are filled in as they are added.
package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
)

// Ensure the implementation satisfies the framework interface at compile time.
var _ provider.Provider = (*Provider)(nil)

type Provider struct {
	version string
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
	// Filled in with the endpoint + token attributes.
	resp.Schema = schema.Schema{}
}

func (p *Provider) Configure(_ context.Context, _ provider.ConfigureRequest, _ *provider.ConfigureResponse) {
	// Resolves config/env, constructs the API client, and shares it.
}

func (p *Provider) Resources(_ context.Context) []func() resource.Resource {
	// Registered here as resources are added (uptimepage_target, ...).
	return nil
}

func (p *Provider) DataSources(_ context.Context) []func() datasource.DataSource {
	return nil
}

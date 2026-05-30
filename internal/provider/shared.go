package provider

import (
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/uptimepage/terraform-provider-uptimepage/internal/client"
)

// validateDiscriminatedBlock enforces that exactly the nested block named by
// kind is set under base. Shared by the target check and the channel config.
func validateDiscriminatedBlock(base path.Path, kind string, present map[string]bool, diags *diag.Diagnostics) {
	if kind != "" && !present[kind] {
		diags.AddAttributeError(base.AtName(kind), "Missing block",
			fmt.Sprintf("type = %q requires the %q block.", kind, kind))
	}
	for k, set := range present {
		if set && k != kind {
			diags.AddAttributeError(base.AtName(k), "Unexpected block",
				fmt.Sprintf("The %q block is set but type = %q.", k, kind))
		}
	}
}

// alertObjectType is the element type of the alerts list, reused by the schema
// default and any list construction.
var alertObjectType = types.ObjectType{AttrTypes: map[string]attr.Type{
	"channel_id":      types.StringType,
	"after_failures":  types.Int64Type,
	"notify_recovery": types.BoolType,
}}

// clientFromProviderData extracts the shared *client.Client set by the provider
// Configure, recording a diagnostic on type mismatch. data is nil during the
// framework's early Configure pass; callers treat (nil, true) as "not ready".
func clientFromProviderData(data any, diags *diag.Diagnostics) (*client.Client, bool) {
	if data == nil {
		return nil, true
	}
	c, ok := data.(*client.Client)
	if !ok {
		diags.AddError("Unexpected provider data",
			fmt.Sprintf("Expected *client.Client, got %T.", data))
		return nil, false
	}
	return c, true
}

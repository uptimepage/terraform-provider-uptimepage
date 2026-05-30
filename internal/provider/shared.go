package provider

import (
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/uptimepage/terraform-provider-uptimepage/internal/client"
)

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

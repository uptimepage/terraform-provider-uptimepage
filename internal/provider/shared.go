package provider

import (
	"context"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework-validators/setvalidator"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
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
	"channel_id": types.StringType,
}}

var regionPolicyObjectType = types.ObjectType{AttrTypes: map[string]attr.Type{
	"mode":  types.StringType,
	"count": types.Int64Type,
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

// trimmedValidator refuses a blank value or one with whitespace at either end:
// the API trims it, so it would read back as another value and fail the apply.
type trimmedValidator struct{}

func (trimmedValidator) Description(context.Context) string {
	return "must not be blank or start or end with whitespace"
}

func (v trimmedValidator) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

func (v trimmedValidator) ValidateString(ctx context.Context, req validator.StringRequest, resp *validator.StringResponse) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}
	s := req.ConfigValue.ValueString()
	if s == "" || strings.TrimSpace(s) != s {
		resp.Diagnostics.AddAttributeError(req.Path, "Invalid Attribute Value",
			fmt.Sprintf("%q %s.", s, v.Description(ctx)))
	}
}

// tagValidators are the plan-time half of the API's tag rules.
func tagValidators() []validator.Set {
	return []validator.Set{setvalidator.ValueStringsAre(trimmedValidator{})}
}

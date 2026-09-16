package provider

import (
	"regexp"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
)

var lowercaseUUID = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

// uuidValidator refuses any spelling the API would not echo back unchanged.
// The server parses ids case-insensitively but stores and returns them in
// lowercase, so an uppercase id applies and then fails the post-apply
// consistency check on every run. Only for ids, never for a secret: the
// diagnostic prints the rejected value.
func uuidValidator() validator.String {
	return stringvalidator.RegexMatches(lowercaseUUID, "must be a lowercase UUID")
}

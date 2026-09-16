package provider

import "github.com/hashicorp/terraform-plugin-go/tftypes"

// rawWith builds a whole-resource value with one attribute set and every other
// one null, which is all a plan modifier under test ever reads.
func rawWith(objType tftypes.Object, name string, value tftypes.Value) tftypes.Value {
	attrs := map[string]tftypes.Value{}
	for n, typ := range objType.AttributeTypes {
		attrs[n] = tftypes.NewValue(typ, nil)
	}
	attrs[name] = value
	return tftypes.NewValue(objType, attrs)
}

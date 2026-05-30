package provider

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
)

// TestProviderSchema_Valid drives GetProviderSchema, which validates every
// provider/resource/data-source schema (Required+Computed conflicts, default vs
// element-type mismatches, nested-attribute validity). Catches schema/model
// drift without needing the acceptance harness.
func TestProviderSchema_Valid(t *testing.T) {
	srv, err := testAccProtoV6["uptimepage"]()
	if err != nil {
		t.Fatalf("build server: %v", err)
	}
	resp, err := srv.GetProviderSchema(context.Background(), &tfprotov6.GetProviderSchemaRequest{})
	if err != nil {
		t.Fatalf("GetProviderSchema: %v", err)
	}
	for _, d := range resp.Diagnostics {
		if d.Severity == tfprotov6.DiagnosticSeverityError {
			t.Errorf("schema diagnostic: %s — %s", d.Summary, d.Detail)
		}
	}
	for _, name := range []string{"uptimepage_target", "uptimepage_notification_channel"} {
		if _, ok := resp.ResourceSchemas[name]; !ok {
			t.Errorf("%s resource schema missing", name)
		}
	}
	if _, ok := resp.DataSourceSchemas["uptimepage_target"]; !ok {
		t.Error("uptimepage_target data source schema missing")
	}
}

//go:build tools

// Package tools pins the codegen/doc tooling so `go run` uses a locked version.
package tools

import (
	_ "github.com/hashicorp/terraform-plugin-docs/cmd/tfplugindocs"
)

package main

import (
	"context"
	"flag"
	"log"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"

	"github.com/uptimepage/terraform-provider-uptimepage/internal/provider"
)

// version is set via -ldflags at release time (goreleaser).
var version = "dev"

func main() {
	var debug bool
	flag.BoolVar(&debug, "debug", false, "run with support for debuggers like delve")
	flag.Parse()

	err := providerserver.Serve(context.Background(), provider.New(version), providerserver.ServeOpts{
		Address: "registry.terraform.io/uptimepage/uptimepage",
		Debug:   debug,
	})
	if err != nil {
		log.Fatal(err)
	}
}

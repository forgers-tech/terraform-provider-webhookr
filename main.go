package main

import (
	"context"
	"flag"
	"log"

	"github.com/forgers-tech/terraform-provider-webhookr/internal/provider"
	"github.com/hashicorp/terraform-plugin-framework/providerserver"
)

// version is set at build time via ldflags.
var version = "dev"

func main() {
	var debug bool
	flag.BoolVar(&debug, "debug", false, "run the provider with debugger support")
	flag.Parse()

	err := providerserver.Serve(context.Background(), provider.New(version), providerserver.ServeOpts{
		Address: "registry.terraform.io/forgers-tech/webhookr",
		Debug:   debug,
	})
	if err != nil {
		log.Fatal(err.Error())
	}
}

// Package main is the entrypoint for the function-auto-labeler Crossplane
// composition function. It starts a gRPC server using the function-sdk-go
// helper so the Crossplane composition engine can call RunFunction.
package main

import (
	"github.com/crossplane/function-sdk-go"
	"github.com/crossplane/function-sdk-go/logging"
)

func main() {
	log, err := logging.NewLogger(logging.DefaultConfig())
	if err != nil {
		panic(err)
	}

	// sdk.Serve handles TLS, port binding, and graceful shutdown.
	// The Function struct (defined in fn.go) implements RunFunction.
	if err := sdk.Serve(&Function{log: log},
		sdk.Listen(":9443"),
	); err != nil {
		log.Info("Cannot serve function", "error", err)
	}
}

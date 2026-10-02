package main

import (
	"fmt"
	"os"

	"github.com/canonical/sunbeam-watchtower/internal/adapter/primary/cli"
)

func main() {
	opts := &cli.Options{
		Out:    os.Stdout,
		ErrOut: os.Stderr,
	}

	if err := cli.NewRootCmd(opts).Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

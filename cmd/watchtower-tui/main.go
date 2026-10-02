package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/canonical/sunbeam-watchtower/internal/adapter/primary/tui"
)

func main() {
	var opts tui.Options
	flag.StringVar(&opts.ConfigPath, "config", "", "config file path")
	flag.StringVar(&opts.ServerAddr, "server", "", "server address (http://host:port or unix:///path)")
	flag.BoolVar(&opts.Verbose, "verbose", false, "enable debug logging")
	flag.BoolVar(&opts.NoColor, "no-color", false, "disable colored output")
	flag.Parse()

	opts.In = os.Stdin
	opts.Out = os.Stdout
	opts.ErrOut = os.Stderr

	if err := tui.Run(context.Background(), opts); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

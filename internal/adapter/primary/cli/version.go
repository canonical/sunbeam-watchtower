package cli

import (
	"fmt"

	frontend "github.com/canonical/sunbeam-watchtower/internal/adapter/primary/frontend"
	"github.com/spf13/cobra"
)

// Version is set at build time via -ldflags.
var Version = "dev"

func newVersionCmd(opts *Options) *cobra.Command {
	return withActionID(&cobra.Command{
		Use:   "version",
		Short: "Print the version",
		RunE: func(cmd *cobra.Command, args []string) error {
			styler := newOutputStylerForOptions(opts, opts.Out, opts.Output)
			fmt.Fprintf(opts.Out, "%s %s\n", styler.Section("watchtower"), Version)
			return nil
		},
	}, frontend.ActionVersionShow)
}

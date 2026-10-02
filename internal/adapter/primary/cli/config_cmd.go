package cli

import (
	frontend "github.com/canonical/sunbeam-watchtower/internal/adapter/primary/frontend"
	"github.com/spf13/cobra"
)

func newConfigCmd(opts *Options) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Manage configuration",
	}

	cmd.AddCommand(newConfigShowCmd(opts))
	cmd.AddCommand(newConfigReloadCmd(opts))
	return cmd
}

func newConfigShowCmd(opts *Options) *cobra.Command {
	return withActionID(&cobra.Command{
		Use:   "show",
		Short: "Display the current configuration",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := opts.Frontend().Config().Show(cmd.Context())
			if err != nil {
				return err
			}

			switch opts.Output {
			case "json":
				return renderJSON(opts.Out, cfg)
			default:
				return renderYAML(opts.Out, cfg)
			}
		},
	}, frontend.ActionConfigShow)
}

func newConfigReloadCmd(opts *Options) *cobra.Command {
	return withActionID(&cobra.Command{
		Use:   "reload",
		Short: "Reload configuration from file",
		RunE: func(cmd *cobra.Command, args []string) error {
			result, err := opts.Frontend().Config().Reload(cmd.Context())
			if err != nil {
				return err
			}

			switch opts.Output {
			case "json":
				return renderJSON(opts.Out, result)
			default:
				return renderYAML(opts.Out, result)
			}
		},
	}, frontend.ActionConfigReload)
}

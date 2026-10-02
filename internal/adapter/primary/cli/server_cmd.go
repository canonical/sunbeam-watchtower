package cli

import (
	"fmt"
	"time"

	frontend "github.com/canonical/sunbeam-watchtower/internal/adapter/primary/frontend"
	"github.com/spf13/cobra"
)

func newServerCmd(opts *Options) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "server",
		Short: "Manage the local persistent Watchtower server",
	}
	cmd.AddCommand(
		newServerStartCmd(opts),
		newServerStatusCmd(opts),
		newServerStopCmd(opts),
	)
	return cmd
}

func newServerStartCmd(opts *Options) *cobra.Command {
	return withActionID(&cobra.Command{
		Use:   "start",
		Short: "Start the local persistent Watchtower server",
		RunE: func(cmd *cobra.Command, args []string) error {
			manager, err := newLocalServerManager(opts)
			if err != nil {
				return err
			}
			styler := newOutputStylerForOptions(opts, opts.Out, opts.Output)
			status, started, err := manager.EnsureRunning(cmd.Context())
			if err != nil {
				return err
			}

			if started {
				fmt.Fprintf(opts.Out, "%s local server at %s (pid %d)\n", styler.Action("Started"), styler.DetailValue("URL", status.Address), status.PID)
			} else {
				fmt.Fprintf(opts.Out, "Local server already %s at %s (pid %d)\n", styler.semantic("running"), styler.DetailValue("URL", status.Address), status.PID)
			}
			if err := writeKeyValue(opts.Out, styler, "Log file", status.LogFile); err != nil {
				return err
			}
			if status.ConfigPath != "" {
				if err := writeKeyValue(opts.Out, styler, "Config", status.ConfigPath); err != nil {
					return err
				}
			}
			return nil
		},
	}, frontend.ActionServerStart)
}

func newServerStatusCmd(opts *Options) *cobra.Command {
	return withActionID(&cobra.Command{
		Use:   "status",
		Short: "Show local persistent server status",
		RunE: func(cmd *cobra.Command, args []string) error {
			manager, err := newLocalServerManager(opts)
			if err != nil {
				return err
			}
			styler := newOutputStylerForOptions(opts, opts.Out, opts.Output)
			status, err := manager.Status(cmd.Context())
			if err != nil {
				return err
			}
			if !status.Running {
				fmt.Fprintf(opts.Out, "Local server %s.\n", styler.Warning("not running"))
				fmt.Fprintf(opts.Out, "%s %s\n", styler.Key("Expected address:"), status.Address)
				if status.StaleSocket {
					fmt.Fprintf(opts.Out, "%s %s\n", styler.Key("Stale socket file detected at"), manager.Paths().Socket)
				}
				if status.StalePIDFile {
					fmt.Fprintf(opts.Out, "%s %s\n", styler.Key("Stale pid file detected at"), manager.Paths().PIDFile)
				}
				return nil
			}
			fmt.Fprintf(opts.Out, "Local server %s at %s (pid %d)\n", styler.semantic("running"), styler.DetailValue("URL", status.Address), status.PID)
			if err := writeKeyValue(opts.Out, styler, "Log file", status.LogFile); err != nil {
				return err
			}
			if status.ConfigPath != "" {
				if err := writeKeyValue(opts.Out, styler, "Config", status.ConfigPath); err != nil {
					return err
				}
			}
			if !status.StartedAt.IsZero() {
				if err := writeKeyValue(opts.Out, styler, "Started", status.StartedAt.Format(time.RFC3339)); err != nil {
					return err
				}
			}
			return nil
		},
	}, frontend.ActionServerStatus)
}

func newServerStopCmd(opts *Options) *cobra.Command {
	return withActionID(&cobra.Command{
		Use:   "stop",
		Short: "Stop the local persistent Watchtower server",
		RunE: func(cmd *cobra.Command, args []string) error {
			manager, err := newLocalServerManager(opts)
			if err != nil {
				return err
			}
			styler := newOutputStylerForOptions(opts, opts.Out, opts.Output)
			stopped, err := manager.Stop(cmd.Context())
			if err != nil {
				return err
			}
			if !stopped {
				fmt.Fprintf(opts.Out, "Local server %s. Cleaned up any stale local server files.\n", styler.Warning("is not running"))
				return nil
			}
			fmt.Fprintf(opts.Out, "%s local server.\n", styler.Action("Stopped"))
			return nil
		},
	}, frontend.ActionServerStop)
}

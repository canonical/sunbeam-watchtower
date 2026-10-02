package cli

import (
	frontend "github.com/canonical/sunbeam-watchtower/internal/adapter/primary/frontend"
	"github.com/spf13/cobra"
)

func newOperationCmd(opts *Options) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "operation",
		Aliases: []string{"operations"},
		Short:   "Inspect and control long-running operations",
	}
	cmd.AddCommand(
		newOperationListCmd(opts),
		newOperationShowCmd(opts),
		newOperationEventsCmd(opts),
		newOperationCancelCmd(opts),
	)
	return cmd
}

func newOperationListCmd(opts *Options) *cobra.Command {
	return withActionID(&cobra.Command{
		Use:   "list",
		Short: "List long-running operations",
		RunE: func(cmd *cobra.Command, args []string) error {
			workflow := opts.Frontend().Operations()
			jobs, err := workflow.List(cmd.Context())
			if err != nil {
				return err
			}
			return renderOperationJobs(opts.Out, opts.Output, newOutputStylerForOptions(opts, opts.Out, opts.Output), jobs)
		},
	}, frontend.ActionOperationList)
}

func newOperationShowCmd(opts *Options) *cobra.Command {
	return withActionID(&cobra.Command{
		Use:   "show <id>",
		Short: "Show one operation",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			workflow := opts.Frontend().Operations()
			job, err := workflow.Get(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			return renderOperationJob(opts.Out, opts.Output, newOutputStylerForOptions(opts, opts.Out, opts.Output), job)
		},
	}, frontend.ActionOperationShow)
}

func newOperationEventsCmd(opts *Options) *cobra.Command {
	return withActionID(&cobra.Command{
		Use:   "events <id>",
		Short: "Show the event history for one operation",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			workflow := opts.Frontend().Operations()
			events, err := workflow.Events(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			return renderOperationEvents(opts.Out, opts.Output, newOutputStylerForOptions(opts, opts.Out, opts.Output), events)
		},
	}, frontend.ActionOperationEvents)
}

func newOperationCancelCmd(opts *Options) *cobra.Command {
	return withActionID(&cobra.Command{
		Use:   "cancel <id>",
		Short: "Request cancellation for one operation",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			workflow := opts.Frontend().Operations()
			job, err := workflow.Cancel(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			return renderOperationJob(opts.Out, opts.Output, newOutputStylerForOptions(opts, opts.Out, opts.Output), job)
		},
	}, frontend.ActionOperationCancel)
}

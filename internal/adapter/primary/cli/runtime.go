package cli

import (
	frontend "github.com/canonical/sunbeam-watchtower/internal/adapter/primary/frontend"
	"github.com/spf13/cobra"

	runtimeadapter "github.com/canonical/sunbeam-watchtower/internal/adapter/primary/runtime"
)

func commandNeedsConfig(cmd *cobra.Command) bool {
	return !commandSkipsConfig(cmd)
}

func commandSkipsConfig(cmd *cobra.Command) bool {
	switch cmd.Name() {
	case "version":
		return true
	case "status", "stop":
		return isServerLifecycleCommand(cmd)
	default:
		return false
	}
}

func commandNeedsSession(cmd *cobra.Command) bool {
	switch {
	case cmd.Name() == "version":
		return false
	case cmd.Name() == "serve":
		return false
	case isServerLifecycleCommand(cmd):
		return false
	default:
		return true
	}
}

func commandNeedsApp(cmd *cobra.Command) bool {
	switch {
	case cmd.Name() == "serve":
		return true
	default:
		return false
	}
}

func commandNeedsPersistentServer(cmd *cobra.Command) bool {
	actionID := commandActionID(cmd, nil)
	if actionID != "" {
		desc := frontend.DescribeAction(actionID)
		if desc.RuntimeRequirement == frontend.RuntimePersistentRequired {
			return true
		}
	}

	parent := ""
	if p := cmd.Parent(); p != nil {
		parent = p.Name()
	}

	switch actionID {
	case frontend.ActionBuildTrigger:
		if cmd.Name() == "trigger" {
			async, _ := cmd.Flags().GetBool("async")
			return async
		}
	case frontend.ActionProjectSyncApply, frontend.ActionProjectSyncDryRun:
		async, _ := cmd.Flags().GetBool("async")
		return async
	case frontend.ActionTeamSyncApply, frontend.ActionTeamSyncDryRun:
		async, _ := cmd.Flags().GetBool("async")
		return async
	}

	switch parent {
	case "operation":
		return true
	case "build":
		if cmd.Name() == "trigger" {
			async, _ := cmd.Flags().GetBool("async")
			return async
		}
	case "project":
		if cmd.Name() == "sync" {
			async, _ := cmd.Flags().GetBool("async")
			return async
		}
	case "team":
		if cmd.Name() == "sync" {
			async, _ := cmd.Flags().GetBool("async")
			return async
		}
	}

	return false
}

func isServerLifecycleCommand(cmd *cobra.Command) bool {
	if cmd == nil || cmd.Parent() == nil {
		return false
	}
	return cmd.Parent().Name() == "server"
}

func targetPolicyForCommand(cmd *cobra.Command) runtimeadapter.TargetPolicy {
	switch {
	case commandNeedsPersistentServer(cmd):
		return runtimeadapter.TargetPolicyRequirePersistent
	default:
		return runtimeadapter.TargetPolicyPreferExistingDaemon
	}
}

func newLocalServerManager(opts *Options) (*runtimeadapter.LocalServerManager, error) {
	return runtimeadapter.NewLocalServerManager(runtimeadapter.Options{
		ConfigPath:     opts.ConfigPath,
		ServerAddr:     opts.ServerAddr,
		Verbose:        opts.Verbose,
		Logger:         opts.Logger,
		ExecutablePath: opts.ExecutablePath,
	})
}

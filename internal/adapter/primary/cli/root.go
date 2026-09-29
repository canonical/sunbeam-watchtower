package cli

import (
	"io"
	"log/slog"
	"os"

	"github.com/gboutry/sunbeam-watchtower/internal/adapter/primary/frontend"
	runtimeadapter "github.com/gboutry/sunbeam-watchtower/internal/adapter/primary/runtime"
	"github.com/gboutry/sunbeam-watchtower/internal/app"
	"github.com/gboutry/sunbeam-watchtower/internal/config"
	"github.com/gboutry/sunbeam-watchtower/pkg/client"
	"github.com/spf13/cobra"
)

// Options holds resolved CLI state shared across commands.
type Options struct {
	ConfigPath     string
	Verbose        bool
	Output         string // "table", "json", "yaml"
	NoColor        bool
	Insecure       bool
	AccessMode     runtimeadapter.AccessMode
	Logger         *slog.Logger
	Out            io.Writer
	ErrOut         io.Writer
	ServerAddr     string // external server address (--server / WATCHTOWER_SERVER)
	ExecutablePath string

	Session *runtimeadapter.Session

	Client *client.Client
	App    *app.App

	config         *config.Config
	frontendFacade *frontend.ClientFacade
	frontendClient *client.Client
	frontendApp    *app.App
}

// Frontend returns the shared client-side frontend facade for the current command execution.
func (o *Options) Frontend() *frontend.ClientFacade {
	if o.Session == nil {
		if o.frontendFacade == nil || o.frontendClient != o.Client || o.frontendApp != o.App {
			o.frontendFacade = frontend.NewClientFacade(frontend.NewClientTransport(o.Client), o.App)
			o.frontendClient = o.Client
			o.frontendApp = o.App
		}
		return o.frontendFacade
	}
	return o.Session.Frontend
}

// Application returns the app for the current command execution.
func (o *Options) Application() *app.App {
	if o.Session != nil {
		return o.Session.App
	}
	return o.App
}

// NewRootCmd creates the root watchtower command with all subcommands.
func NewRootCmd(opts *Options) *cobra.Command {
	if opts.Out == nil {
		opts.Out = os.Stdout
	}
	if opts.ErrOut == nil {
		opts.ErrOut = os.Stderr
	}

	root := &cobra.Command{
		Use:           "watchtower",
		Short:         "Unified dashboard for Sunbeam across GitHub, Launchpad, and Gerrit",
		SilenceUsage:  true,
		SilenceErrors: true,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			// Apply env var defaults for flags not explicitly set.
			runtimeOpts := runtimeadapter.Options{
				ConfigPath: opts.ConfigPath,
				ServerAddr: opts.ServerAddr,
				Verbose:    opts.Verbose,
			}
			runtimeadapter.ApplyEnvDefaults(&runtimeOpts)
			opts.ConfigPath = runtimeOpts.ConfigPath
			opts.ServerAddr = runtimeOpts.ServerAddr
			opts.Verbose = runtimeOpts.Verbose
			if v := os.Getenv("WATCHTOWER_CONFIG"); v != "" && opts.ConfigPath == "" {
				opts.ConfigPath = v
			}
			if v := os.Getenv("WATCHTOWER_VERBOSE"); v != "" && !opts.Verbose {
				opts.Verbose = v == "1" || v == "true" || v == "TRUE" || v == "True"
			}
			if v := os.Getenv("WATCHTOWER_OUTPUT"); v != "" && opts.Output == "table" {
				opts.Output = v
			}
			if v := os.Getenv("WATCHTOWER_NO_COLOR"); v != "" && !opts.NoColor {
				opts.NoColor = v == "1" || v == "true" || v == "TRUE" || v == "True"
			}
			if v := os.Getenv("WATCHTOWER_SERVER"); v != "" && opts.ServerAddr == "" {
				opts.ServerAddr = v
			}

			opts.Logger = runtimeadapter.NewLogger(opts.Verbose, opts.ErrOut)
			if opts.AccessMode == "" {
				opts.AccessMode = runtimeadapter.AccessModeFull
			}
			if err := preflightCommandAccess(opts, cmd, args); err != nil {
				return err
			}

			if commandNeedsSession(cmd) {
				session, err := runtimeadapter.NewSession(cmd.Context(), runtimeadapter.Options{
					ConfigPath:     opts.ConfigPath,
					ServerAddr:     opts.ServerAddr,
					Verbose:        opts.Verbose,
					Insecure:       opts.Insecure,
					Logger:         opts.Logger,
					LogWriter:      opts.ErrOut,
					ExecutablePath: opts.ExecutablePath,
					TargetPolicy:   targetPolicyForCommand(cmd),
					AccessMode:     opts.AccessMode,
				})
				if err != nil {
					return err
				}
				opts.Session = session
				opts.config = session.Config.LocalConfig()
				opts.ServerAddr = session.Target().Address
				return nil
			}

			if commandNeedsConfig(cmd) {
				cfg, err := config.Load(opts.ConfigPath)
				if err != nil {
					return err
				}
				opts.config = cfg
			}

			if commandNeedsApp(cmd) {
				opts.App = app.NewAppWithOptions(opts.config, opts.Logger, app.Options{
					RuntimeMode: app.RuntimeModePersistent,
					ConfigPath:  opts.ConfigPath,
				})
			}
			return nil
		},
		PersistentPostRunE: func(cmd *cobra.Command, args []string) error {
			if opts.Session != nil {
				err := opts.Session.Close()
				opts.Session = nil
				opts.config = nil
				return err
			}
			if opts.App != nil {
				err := opts.App.Close()
				opts.App = nil
				opts.config = nil
				return err
			}
			return nil
		},
	}

	root.PersistentFlags().StringVar(&opts.ConfigPath, "config", "", "config file path")
	root.PersistentFlags().BoolVar(&opts.Verbose, "verbose", false, "enable debug logging")
	root.PersistentFlags().StringVarP(&opts.Output, "output", "o", "table", "output format: table, json, yaml")
	root.PersistentFlags().BoolVar(&opts.NoColor, "no-color", false, "disable colored output")
	root.PersistentFlags().BoolVar(&opts.Insecure, "insecure", false, "allow sending bearer tokens over cleartext HTTP to remote servers")
	root.PersistentFlags().StringVar(&opts.ServerAddr, "server", "", "server address (http://host:port or unix:///path)")

	root.AddGroup(
		&cobra.Group{ID: "workflow", Title: "Workflows"},
		&cobra.Group{ID: "meta", Title: "Meta Commands"},
	)
	root.SetHelpCommandGroupID("meta")
	root.SetCompletionCommandGroupID("meta")

	root.AddCommand(
		withGroupID(newReviewCmd(opts), "workflow"),
		withGroupID(newCommitCmd(opts), "workflow"),
		withGroupID(newBugCmd(opts), "workflow"),
		withGroupID(newSRUCmd(opts), "workflow"),
		withGroupID(newBuildCmd(opts), "workflow"),
		withGroupID(newReleasesCmd(opts), "workflow"),
		withGroupID(newProjectCmd(opts), "workflow"),
		withGroupID(newTeamCmd(opts), "workflow"),
		withGroupID(newPackagesCmd(opts), "workflow"),
	)
	root.AddCommand(
		withGroupID(newVersionCmd(opts), "meta"),
		withGroupID(newConfigCmd(opts), "meta"),
		withGroupID(newAuthCmd(opts), "meta"),
		withGroupID(newOperationCmd(opts), "meta"),
		withGroupID(newCacheCmd(opts), "meta"),
		withGroupID(newServeCmd(opts), "meta"),
		withGroupID(newServerCmd(opts), "meta"),
	)

	return root
}

func withGroupID(cmd *cobra.Command, groupID string) *cobra.Command {
	cmd.GroupID = groupID
	return cmd
}

package cli

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/canonical/sunbeam-watchtower/internal/adapter/primary/api"
	frontend "github.com/canonical/sunbeam-watchtower/internal/adapter/primary/frontend"
	runtimeadapter "github.com/canonical/sunbeam-watchtower/internal/adapter/primary/runtime"
	"github.com/spf13/cobra"
)

func newServeCmd(opts *Options) *cobra.Command {
	var listen string

	cmd := withActionID(&cobra.Command{
		Use:   "serve",
		Short: "Start the HTTP API server",
		Long:  "Start the Watchtower HTTP API server. The server provides a RESTful API for all watchtower operations and serves an OpenAPI spec at /openapi.json.",
		RunE: func(cmd *cobra.Command, args []string) error {
			var serverOpts api.ServerOptions

			switch {
			case strings.HasPrefix(listen, "unix://"):
				serverOpts.UnixSocket = strings.TrimPrefix(listen, "unix://")
			case strings.HasPrefix(listen, "tcp://"):
				serverOpts.ListenAddr = strings.TrimPrefix(listen, "tcp://")
			default:
				serverOpts.ListenAddr = listen
			}

			// Resolve auth token for TCP listeners.
			if serverOpts.UnixSocket == "" {
				authToken := opts.Application().GetConfig().AuthToken
				if authToken == "" {
					generated, err := api.GenerateToken()
					if err != nil {
						return fmt.Errorf("generating auth token: %w", err)
					}
					authToken = generated

					// Write to well-known path for local clients.
					tokenDir, err := os.UserHomeDir()
					if err == nil {
						tokenPath := filepath.Join(tokenDir, ".config", "sunbeam-watchtower", "server.token")
						if err := os.MkdirAll(filepath.Dir(tokenPath), 0o755); err == nil {
							if err := os.WriteFile(tokenPath, []byte(authToken), 0o600); err != nil {
								opts.Logger.Warn("failed to write server token file", "error", err)
							} else {
								opts.Logger.Info("auth token written", "path", tokenPath)
							}
						}
					}
				}
				serverOpts.AuthToken = authToken
				opts.Logger.Info("TCP authentication enabled")
			}

			srv := runtimeadapter.NewConfiguredServer(opts.Logger, opts.Application(), serverOpts)

			if err := srv.Start(); err != nil {
				return fmt.Errorf("starting server: %w", err)
			}

			application := opts.Application()

			addr := serverOpts.ListenAddr
			if addr == "" {
				addr = "127.0.0.1:8472"
			}
			if serverOpts.UnixSocket == "" {
				opts.Logger.Info("OpenAPI spec available", "url", "http://"+addr+"/openapi.json")
			}

			// Start config file watcher if a config path is known.
			configPath := application.ConfigPath()
			if configPath == "" {
				configPath = opts.ConfigPath
			}
			var cw *runtimeadapter.ConfigWatcher
			if configPath != "" {
				var err error
				cw, err = runtimeadapter.NewConfigWatcher(configPath, application.ReloadConfig, opts.Logger)
				if err != nil {
					opts.Logger.Warn("failed to start config file watcher", "error", err)
				}
			}

			// Register SIGHUP handler for manual config reload.
			sighup := make(chan os.Signal, 1)
			signal.Notify(sighup, syscall.SIGHUP)
			go func() {
				for range sighup {
					opts.Logger.Info("received SIGHUP, reloading configuration")
					if reloadPath := application.ConfigPath(); reloadPath != "" {
						if err := application.ReloadConfig(reloadPath); err != nil {
							opts.Logger.Error("SIGHUP config reload failed", "error", err)
						}
					} else {
						opts.Logger.Warn("SIGHUP received but no config path is set")
					}
				}
			}()

			ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
			defer stop()

			<-ctx.Done()

			opts.Logger.Info("shutting down gracefully")

			signal.Stop(sighup)
			close(sighup)
			if cw != nil {
				cw.Stop()
			}

			shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()

			return srv.Shutdown(shutdownCtx)
		},
	}, frontend.ActionServeStart)

	cmd.Flags().StringVar(&listen, "listen", "127.0.0.1:8472", "listen address (tcp://host:port, unix:///path, or host:port)")
	return cmd
}

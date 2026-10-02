// SPDX-FileCopyrightText: 2026 - gboutry
// SPDX-License-Identifier: Apache-2.0

package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/canonical/sunbeam-watchtower/internal/adapter/primary/api"
	frontend "github.com/canonical/sunbeam-watchtower/internal/adapter/primary/frontend"
	"github.com/canonical/sunbeam-watchtower/internal/app"
	"github.com/canonical/sunbeam-watchtower/internal/config"
	"github.com/canonical/sunbeam-watchtower/pkg/client"
)

// TargetKind identifies how one frontend session reaches the API.
type TargetKind string

const (
	TargetKindEmbedded TargetKind = "embedded"
	TargetKindDaemon   TargetKind = "daemon"
	TargetKindRemote   TargetKind = "remote"
)

// TargetPolicy controls how a frontend session resolves its API target.
type TargetPolicy string

const (
	TargetPolicyPreferEmbedded       TargetPolicy = "prefer_embedded"
	TargetPolicyPreferExistingDaemon TargetPolicy = "prefer_existing_daemon"
	TargetPolicyRequirePersistent    TargetPolicy = "require_persistent"
)

// AccessMode controls whether a frontend session can run mutating actions.
type AccessMode string

const (
	AccessModeFull     AccessMode = "full"
	AccessModeReadOnly AccessMode = "read_only"
)

// Options controls runtime/session construction for frontends.
type Options struct {
	ConfigPath     string
	ServerAddr     string
	Verbose        bool
	Insecure       bool
	Logger         *slog.Logger
	LogWriter      io.Writer
	ExecutablePath string
	TargetPolicy   TargetPolicy
	AccessMode     AccessMode
}

// ApplyEnvDefaults applies WATCHTOWER_* environment variables as defaults.
func ApplyEnvDefaults(opts *Options) {
	if opts == nil {
		return
	}
	if v := os.Getenv("WATCHTOWER_CONFIG"); v != "" && opts.ConfigPath == "" {
		opts.ConfigPath = v
	}
	if v := os.Getenv("WATCHTOWER_VERBOSE"); v != "" && !opts.Verbose {
		opts.Verbose = v == "1" || v == "true" || v == "TRUE" || v == "True"
	}
	if v := os.Getenv("WATCHTOWER_SERVER"); v != "" && opts.ServerAddr == "" {
		opts.ServerAddr = v
	}
}

// NewLogger creates the default frontend logger.
func NewLogger(verbose bool, w io.Writer) *slog.Logger {
	if w == nil {
		w = io.Discard
	}
	level := slog.LevelInfo
	if verbose {
		level = slog.LevelDebug
	}
	return slog.New(slog.NewTextHandler(w, &slog.HandlerOptions{Level: level}))
}

// LocalServerPaths describes the local daemon files/socket.
type LocalServerPaths struct {
	Dir       string
	Socket    string
	PIDFile   string
	Metadata  string
	LogFile   string
	SocketURI string
}

// LocalServerMetadata stores persisted daemon metadata.
type LocalServerMetadata struct {
	PID            int       `json:"pid"`
	Address        string    `json:"address"`
	StartedAt      time.Time `json:"started_at"`
	LogFile        string    `json:"log_file"`
	ConfigPath     string    `json:"config_path,omitempty"`
	ExecutablePath string    `json:"executable_path,omitempty"`
}

// LocalServerStatus describes the currently discovered daemon state.
type LocalServerStatus struct {
	Address         string
	PID             int
	Running         bool
	LogFile         string
	ConfigPath      string
	StartedAt       time.Time
	StalePIDFile    bool
	StaleSocket     bool
	MetadataPresent bool
}

// LocalServerManager manages one local persistent daemon.
type LocalServerManager struct {
	logger         *slog.Logger
	configPath     string
	verbose        bool
	executablePath string
	paths          LocalServerPaths
}

// ResolveLocalServerPaths resolves the local daemon file layout.
func ResolveLocalServerPaths() (LocalServerPaths, error) {
	var dir string
	if runtimeDir := os.Getenv("XDG_RUNTIME_DIR"); runtimeDir != "" {
		dir = filepath.Join(runtimeDir, "sunbeam-watchtower")
	} else {
		cacheDir, err := app.ResolveCacheDir()
		if err != nil {
			return LocalServerPaths{}, err
		}
		dir = filepath.Join(cacheDir, "run")
	}

	return LocalServerPaths{
		Dir:       dir,
		Socket:    filepath.Join(dir, "watchtower.sock"),
		PIDFile:   filepath.Join(dir, "watchtower.pid"),
		Metadata:  filepath.Join(dir, "watchtower.json"),
		LogFile:   filepath.Join(dir, "watchtower.log"),
		SocketURI: "unix://" + filepath.Join(dir, "watchtower.sock"),
	}, nil
}

// NewLocalServerManager creates a local daemon manager.
func NewLocalServerManager(opts Options) (*LocalServerManager, error) {
	paths, err := ResolveLocalServerPaths()
	if err != nil {
		return nil, err
	}

	executablePath := opts.ExecutablePath
	if executablePath == "" {
		executablePath, err = os.Executable()
		if err != nil {
			return nil, fmt.Errorf("resolve executable path: %w", err)
		}
	}

	logger := opts.Logger
	if logger == nil {
		logger = NewLogger(opts.Verbose, opts.LogWriter)
	}

	return &LocalServerManager{
		logger:         logger,
		configPath:     opts.ConfigPath,
		verbose:        opts.Verbose,
		executablePath: executablePath,
		paths:          paths,
	}, nil
}

// Paths returns the resolved daemon paths.
func (m *LocalServerManager) Paths() LocalServerPaths {
	if m == nil {
		return LocalServerPaths{}
	}
	return m.paths
}

// Status returns the current daemon status.
func (m *LocalServerManager) Status(ctx context.Context) (LocalServerStatus, error) {
	status := LocalServerStatus{
		Address: m.paths.SocketURI,
		LogFile: m.paths.LogFile,
	}

	if metadata, err := ReadLocalServerMetadata(m.paths.Metadata); err == nil {
		status.MetadataPresent = true
		if metadata.Address != "" {
			status.Address = metadata.Address
		}
		if metadata.LogFile != "" {
			status.LogFile = metadata.LogFile
		}
		status.ConfigPath = metadata.ConfigPath
		status.StartedAt = metadata.StartedAt
		if metadata.PID != 0 {
			status.PID = metadata.PID
		}
	}

	if pid, err := readPIDFile(m.paths.PIDFile); err == nil {
		status.PID = pid
	}

	healthy := client.NewClient(m.paths.SocketURI).Health(ctx) == nil
	if !healthy {
		if _, err := os.Stat(m.paths.Socket); err == nil {
			status.StaleSocket = true
		}
		if _, err := os.Stat(m.paths.PIDFile); err == nil {
			status.StalePIDFile = true
		}
		return status, nil
	}

	status.Running = true
	return status, nil
}

// EnsureRunning starts the local daemon if needed.
func (m *LocalServerManager) EnsureRunning(ctx context.Context) (LocalServerStatus, bool, error) {
	status, err := m.Status(ctx)
	if err != nil {
		return LocalServerStatus{}, false, err
	}
	if status.Running {
		return status, false, nil
	}

	if err := os.MkdirAll(m.paths.Dir, 0o700); err != nil {
		return LocalServerStatus{}, false, fmt.Errorf("create runtime dir: %w", err)
	}
	if err := m.cleanupStaleFiles(status); err != nil {
		return LocalServerStatus{}, false, err
	}

	logFile, err := os.OpenFile(m.paths.LogFile, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return LocalServerStatus{}, false, fmt.Errorf("open server log: %w", err)
	}
	defer logFile.Close()

	args := []string{"serve", "--listen", m.paths.SocketURI}
	if m.configPath != "" {
		args = append(args, "--config", m.configPath)
	}
	if m.verbose {
		args = append(args, "--verbose")
	}

	cmd := exec.Command(m.executablePath, args...)
	cmd.Stdin = nil
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}

	if err := cmd.Start(); err != nil {
		return LocalServerStatus{}, false, fmt.Errorf("start local server process: %w", err)
	}
	if err := os.WriteFile(m.paths.PIDFile, []byte(strconv.Itoa(cmd.Process.Pid)), 0o600); err != nil {
		_ = cmd.Process.Kill()
		return LocalServerStatus{}, false, fmt.Errorf("write server pid file: %w", err)
	}
	if err := WriteLocalServerMetadata(m.paths.Metadata, LocalServerMetadata{
		PID:            cmd.Process.Pid,
		Address:        m.paths.SocketURI,
		StartedAt:      time.Now().UTC(),
		LogFile:        m.paths.LogFile,
		ConfigPath:     m.configPath,
		ExecutablePath: m.executablePath,
	}); err != nil {
		_ = cmd.Process.Kill()
		_ = os.Remove(m.paths.PIDFile)
		return LocalServerStatus{}, false, fmt.Errorf("write server metadata: %w", err)
	}
	_ = cmd.Process.Release()

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		status, err = m.Status(ctx)
		if err != nil {
			return LocalServerStatus{}, false, err
		}
		if status.Running {
			return status, true, nil
		}
		time.Sleep(100 * time.Millisecond)
	}

	return LocalServerStatus{}, false, fmt.Errorf("local server did not become healthy; see %s", m.paths.LogFile)
}

// Stop stops the local daemon.
func (m *LocalServerManager) Stop(ctx context.Context) (bool, error) {
	status, err := m.Status(ctx)
	if err != nil {
		return false, err
	}
	if !status.Running {
		if err := m.cleanupStaleFiles(status); err != nil {
			return false, err
		}
		return false, nil
	}
	if status.PID == 0 {
		return false, fmt.Errorf("local server is running at %s but %s is missing", status.Address, m.paths.PIDFile)
	}

	proc, err := os.FindProcess(status.PID)
	if err != nil {
		return false, fmt.Errorf("find local server process: %w", err)
	}
	if err := proc.Signal(syscall.SIGTERM); err != nil {
		return false, fmt.Errorf("signal local server: %w", err)
	}

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		status, err = m.Status(ctx)
		if err != nil {
			return false, err
		}
		if !status.Running {
			if err := m.cleanupStaleFiles(status); err != nil {
				return false, err
			}
			return true, nil
		}
		time.Sleep(100 * time.Millisecond)
	}

	return false, fmt.Errorf("local server did not stop within timeout")
}

func (m *LocalServerManager) cleanupStaleFiles(status LocalServerStatus) error {
	if status.StaleSocket || status.StalePIDFile || status.MetadataPresent {
		if err := os.Remove(m.paths.Socket); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("remove stale server socket: %w", err)
		}
		if err := os.Remove(m.paths.PIDFile); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("remove stale server pid file: %w", err)
		}
		if err := os.Remove(m.paths.Metadata); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("remove stale server metadata: %w", err)
		}
	}
	return nil
}

// ReadLocalServerMetadata reads daemon metadata from disk.
func ReadLocalServerMetadata(path string) (*LocalServerMetadata, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var metadata LocalServerMetadata
	if err := json.Unmarshal(content, &metadata); err != nil {
		return nil, fmt.Errorf("parse server metadata %s: %w", path, err)
	}
	return &metadata, nil
}

// WriteLocalServerMetadata persists daemon metadata to disk.
func WriteLocalServerMetadata(path string, metadata LocalServerMetadata) error {
	content, err := json.MarshalIndent(metadata, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal server metadata: %w", err)
	}
	if err := os.WriteFile(path, content, 0o600); err != nil {
		return fmt.Errorf("write server metadata %s: %w", path, err)
	}
	return nil
}

func readPIDFile(path string) (int, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return 0, err
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(content)))
	if err != nil {
		return 0, fmt.Errorf("parse pid file %s: %w", path, err)
	}
	return pid, nil
}

// NewConfiguredServer wires one HTTP API server from the application.
func NewConfiguredServer(logger *slog.Logger, application *app.App, serverOpts api.ServerOptions) *api.Server {
	if application != nil {
		if telemetry, err := application.Telemetry(context.Background()); err == nil && telemetry != nil {
			serverOpts.Middleware = telemetry.Middleware
			if wrapped := telemetry.Logger(logger); wrapped != nil {
				logger = wrapped
				application.Logger = wrapped
			}
		} else if err != nil && logger != nil {
			logger.Warn("failed to initialize telemetry", "error", err)
		}
	}
	srv := api.NewServer(logger, serverOpts)
	api.RegisterAuthAPI(srv.API(), application)
	api.RegisterPackagesAPI(srv.API(), application)
	api.RegisterBugsAPI(srv.API(), application)
	api.RegisterSRUAPI(srv.API(), application)
	api.RegisterCacheAPI(srv.API(), application)
	api.RegisterConfigAPI(srv.API(), application)
	api.RegisterReviewsAPI(srv.API(), application)
	api.RegisterCommitsAPI(srv.API(), application)
	api.RegisterBuildsAPI(srv.API(), application)
	api.RegisterReleasesAPI(srv.API(), application)
	api.RegisterProjectsAPI(srv.API(), application)
	api.RegisterTeamAPI(srv.API(), application)
	api.RegisterOperationsAPI(srv.API(), application)
	return srv
}

// TargetInfo describes the frontend's current API target.
type TargetInfo struct {
	Kind        TargetKind
	Address     string
	LogFile     string
	ConfigPath  string
	StartedAt   time.Time
	PID         int
	Remote      bool
	CanUpgrade  bool
	Description string
}

// Session owns the local app plus current API target for a frontend.
type Session struct {
	Config   *ConfigResolver
	Logger   *slog.Logger
	App      *app.App
	Client   *client.Client
	Frontend *frontend.ClientFacade

	accessMode  AccessMode
	token       string
	target      TargetInfo
	manager     *LocalServerManager
	embeddedSrv *api.Server
	opts        Options
}

// GetConfig returns the effective configuration for this session.
// It resolves lazily: if a remote client is available, the server config
// is fetched; otherwise the local config (if any) is returned.
func (s *Session) GetConfig(ctx context.Context) (*config.Config, error) {
	if s == nil || s.Config == nil {
		return nil, errors.New("no session or config resolver available")
	}
	return s.Config.Resolve(ctx)
}

// NewSession creates one frontend session.
func NewSession(ctx context.Context, opts Options) (*Session, error) {
	ApplyEnvDefaults(&opts)
	if opts.AccessMode == "" {
		opts.AccessMode = AccessModeFull
	}
	logger := opts.Logger
	if logger == nil {
		logger = NewLogger(opts.Verbose, opts.LogWriter)
	}

	// Tolerant config load — nil is okay for remote/daemon targets.
	cfg, _ := config.Load(opts.ConfigPath)

	// Resolve server address from config if not provided via flag/env.
	if opts.ServerAddr == "" && cfg != nil && cfg.ServerAddress != "" {
		opts.ServerAddr = cfg.ServerAddress
	}

	// Resolve token from WATCHTOWER_TOKEN env or config.
	token := os.Getenv("WATCHTOWER_TOKEN")
	if token == "" && cfg != nil {
		token = cfg.ServerToken
	}

	session := &Session{
		Logger:     logger,
		accessMode: opts.AccessMode,
		token:      token,
		opts:       opts,
	}

	// Fast path: explicit remote target — skip App and LocalServerManager.
	if opts.ServerAddr != "" {
		if token != "" {
			c, err := client.NewClientWithToken(opts.ServerAddr, token, opts.Insecure)
			if err != nil {
				return nil, err
			}
			session.Client = c
		} else {
			session.Client = client.NewClient(opts.ServerAddr)
		}
		session.Config = NewConfigResolver(cfg, session.Client)
		session.target = TargetInfo{
			Kind:        TargetKindRemote,
			Address:     opts.ServerAddr,
			Remote:      true,
			Description: "remote server",
		}
		session.Frontend = frontend.NewClientFacade(frontend.NewClientTransport(session.Client), nil)
		return session, nil
	}

	// Local path: need manager to discover/start daemon.
	manager, err := NewLocalServerManager(Options{
		ConfigPath:     opts.ConfigPath,
		ServerAddr:     opts.ServerAddr,
		Verbose:        opts.Verbose,
		Logger:         logger,
		LogWriter:      opts.LogWriter,
		ExecutablePath: opts.ExecutablePath,
	})
	if err != nil {
		return nil, err
	}
	session.manager = manager

	status, err := manager.Status(ctx)
	if err != nil {
		return nil, err
	}

	switch opts.TargetPolicy {
	case TargetPolicyPreferEmbedded:
		// Embedded mode needs a full local config and App.
		if cfg == nil {
			return nil, errors.New("embedded mode requires a configuration file")
		}
		application := app.NewAppWithOptions(cfg, logger, app.Options{RuntimeMode: app.RuntimeModeEphemeral, ConfigPath: opts.ConfigPath})
		session.App = application
		session.Config = NewConfigResolver(cfg, nil)
		if err := session.startEmbeddedTarget(); err != nil {
			_ = application.Close()
			return nil, err
		}

	case TargetPolicyPreferExistingDaemon:
		if status.Running {
			session.useDaemonTarget(status, token)
			session.Config = NewConfigResolver(cfg, session.Client)
			return session, nil
		}
		// Fall back to embedded.
		if cfg == nil {
			return nil, errors.New("no running daemon found and embedded mode requires a configuration file")
		}
		application := app.NewAppWithOptions(cfg, logger, app.Options{RuntimeMode: app.RuntimeModeEphemeral, ConfigPath: opts.ConfigPath})
		session.App = application
		session.Config = NewConfigResolver(cfg, nil)
		if err := session.startEmbeddedTarget(); err != nil {
			_ = application.Close()
			return nil, err
		}

	case TargetPolicyRequirePersistent:
		if status.Running {
			session.useDaemonTarget(status, token)
			session.Config = NewConfigResolver(cfg, session.Client)
			return session, nil
		}
		if cfg == nil {
			return nil, errors.New("cannot auto-start daemon without full configuration — provide a complete config file or connect to an existing server (--server)")
		}
		application := app.NewAppWithOptions(cfg, logger, app.Options{RuntimeMode: app.RuntimeModeEphemeral, ConfigPath: opts.ConfigPath})
		session.App = application
		status, _, err = manager.EnsureRunning(ctx)
		if err != nil {
			_ = application.Close()
			return nil, err
		}
		session.useDaemonTarget(status, token)
		session.Config = NewConfigResolver(cfg, session.Client)

	default:
		return nil, fmt.Errorf("runtime session requires a target policy")
	}

	return session, nil
}

// Target returns the current target metadata.
func (s *Session) Target() TargetInfo {
	if s == nil {
		return TargetInfo{}
	}
	return s.target
}

// AccessMode reports how the session should gate mutating actions.
func (s *Session) AccessMode() AccessMode {
	if s == nil || s.accessMode == "" {
		return AccessModeFull
	}
	return s.accessMode
}

// LocalServerStatus reports the current local daemon status.
func (s *Session) LocalServerStatus(ctx context.Context) (LocalServerStatus, error) {
	if s == nil || s.manager == nil {
		return LocalServerStatus{}, errors.New("runtime session does not have a local server manager")
	}
	return s.manager.Status(ctx)
}

// ActionAccessError reports that a mutating action was denied in read-only mode.
type ActionAccessError struct {
	Action     frontend.ActionDescriptor
	AccessMode AccessMode
}

func (e *ActionAccessError) Error() string {
	return fmt.Sprintf("action %q requires an explicit override in %s mode", e.Action.ID, e.AccessMode)
}

// CheckActionAccess validates one action against the current access mode.
func CheckActionAccess(mode AccessMode, actionID frontend.ActionID, override bool) error {
	if mode == "" {
		mode = AccessModeFull
	}
	if mode != AccessModeReadOnly {
		return nil
	}
	if frontend.IsAllowedInReadOnlyMode(actionID, override) {
		return nil
	}
	return &ActionAccessError{
		Action:     frontend.DescribeAction(actionID),
		AccessMode: mode,
	}
}

// UpgradeToPersistent switches the session from embedded mode to the local daemon.
func (s *Session) UpgradeToPersistent(ctx context.Context) error {
	if s == nil {
		return errors.New("runtime session is nil")
	}
	if s.target.Kind == TargetKindDaemon {
		return nil
	}
	if s.target.Kind == TargetKindRemote {
		return errors.New("remote sessions cannot be upgraded to the local daemon")
	}
	status, _, err := s.manager.EnsureRunning(ctx)
	if err != nil {
		return err
	}
	if s.embeddedSrv != nil {
		if err := s.embeddedSrv.Shutdown(context.Background()); err != nil {
			return err
		}
		s.embeddedSrv = nil
	}
	s.useDaemonTarget(status, s.token)
	return nil
}

// Close releases session resources.
func (s *Session) Close() error {
	if s == nil {
		return nil
	}
	var err error
	if s.embeddedSrv != nil {
		err = s.embeddedSrv.Shutdown(context.Background())
	}
	if s.App != nil {
		err = errors.Join(err, s.App.Close())
	}
	return err
}

func (s *Session) startEmbeddedTarget() error {
	srv := NewConfiguredServer(s.Logger, s.App, api.ServerOptions{ListenAddr: "127.0.0.1:0"})
	if err := srv.Start(); err != nil {
		return err
	}
	s.embeddedSrv = srv
	s.Client = client.NewClient("http://" + srv.Addr())
	s.target = TargetInfo{
		Kind:        TargetKindEmbedded,
		Address:     "http://" + srv.Addr(),
		CanUpgrade:  true,
		Description: "embedded session server",
	}
	s.Frontend = frontend.NewClientFacade(frontend.NewClientTransport(s.Client), s.App)
	return nil
}

func (s *Session) useDaemonTarget(status LocalServerStatus, token string) {
	if token != "" {
		// Daemon addresses are unix:// or localhost — transport check always passes.
		c, _ := client.NewClientWithToken(status.Address, token)
		s.Client = c
	} else {
		s.Client = client.NewClient(status.Address)
	}
	s.target = TargetInfo{
		Kind:        TargetKindDaemon,
		Address:     status.Address,
		LogFile:     status.LogFile,
		ConfigPath:  status.ConfigPath,
		StartedAt:   status.StartedAt,
		PID:         status.PID,
		CanUpgrade:  false,
		Description: "local persistent daemon",
	}
	s.Frontend = frontend.NewClientFacade(frontend.NewClientTransport(s.Client), s.App)
}

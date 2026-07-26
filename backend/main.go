// Package main provides the entry point for the BoxBox server.
package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	"github.com/jR4dh3y/BoxBox/backend/internal/config"
	"github.com/jR4dh3y/BoxBox/backend/internal/handler"
	"github.com/jR4dh3y/BoxBox/backend/internal/middleware"
	"github.com/jR4dh3y/BoxBox/backend/internal/model"
	"github.com/jR4dh3y/BoxBox/backend/internal/pkg/filesystem"
	"github.com/jR4dh3y/BoxBox/backend/internal/service"
	"github.com/jR4dh3y/BoxBox/backend/internal/static"
	"github.com/jR4dh3y/BoxBox/backend/internal/websocket"
)

func main() {
	// Parse command line flags
	configPath := flag.String("config", "", "Path to configuration file")
	flag.Parse()

	// Configure zerolog
	zerolog.TimeFieldFormat = zerolog.TimeFormatUnix
	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr, TimeFormat: time.RFC3339})

	// Load configuration
	loadResult, err := config.LoadWithReport(*configPath)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to load configuration")
	}
	cfg := loadResult.Config

	for _, warning := range loadResult.Warnings {
		log.Warn().
			Str("legacy", warning.Legacy).
			Str("replacement", warning.Replacement).
			Msg(warning.Message)
	}

	hostCount := 0
	if cfg.DockerHosts != nil {
		hostCount = len(cfg.DockerHosts.Hosts)
	}
	log.Info().
		Int("port", cfg.Port).
		Str("host", cfg.Host).
		Int("hosts", hostCount).
		Msg("Configuration loaded")

	// Create context that listens for shutdown signals
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Initialize components
	server, hub, jobService, authService, streamHandler, err := initializeServer(ctx, cfg)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to initialize server")
	}

	// Start WebSocket hub in background
	go hub.Run(ctx)
	log.Info().Msg("WebSocket hub started")

	// Start job service workers
	jobService.Start(ctx)
	log.Info().Msg("Job service started")

	// Start auth service cleanup
	authService.StartCleanup(ctx)
	log.Info().Msg("Auth service cleanup started")

	// Start upload session cleanup
	streamHandler.StartCleanup(ctx)
	log.Info().Msg("Upload session cleanup started")

	// Ensure data directory exists for settings storage
	if err := os.MkdirAll(cfg.DataDir, 0755); err != nil {
		log.Warn().Err(err).Str("path", cfg.DataDir).Msg("Could not create data directory, settings may not persist")
	} else {
		log.Info().Str("path", cfg.DataDir).Msg("Data directory created/verified")
	}

	// Start HTTP server in background
	go func() {
		addr := fmt.Sprintf("%s:%d", cfg.Host, cfg.Port)
		log.Info().Str("addr", addr).Msg("Starting HTTP server")
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal().Err(err).Msg("HTTP server error")
		}
	}()

	// Wait for shutdown signal
	waitForShutdown(cancel, server, jobService, authService, streamHandler)
}

// initializeServer creates and configures all server components
func initializeServer(ctx context.Context, cfg *model.ServerConfig) (*http.Server, *websocket.Hub, service.JobService, service.AuthService, *handler.StreamHandler, error) {
	// Create filesystem abstraction (using real OS filesystem)
	fs := filesystem.NewOsFS()

	// Collect mount points from host configurations
	mountPoints := make([]model.MountPoint, 0)
	if cfg.DockerHosts != nil {
		for _, host := range cfg.DockerHosts.Hosts {
			for name, mp := range host.MountPoints {
				mountPoints = append(mountPoints, model.MountPoint{
					Name:     name,
					Path:     mp.Path,
					ReadOnly: mp.ReadOnly,
				})
			}
		}
	}
	for _, mp := range mountPoints {
		log.Info().Str("path", mp.Path).Str("name", mp.Name).Bool("read_only", mp.ReadOnly).Msg("Mount point")
	}

	// Create WebSocket hub
	hub := websocket.NewHub()

	// Create services
	authService := service.NewAuthService(service.AuthServiceConfig{
		JWTSecret: cfg.JWTSecret,
		Users:     cfg.Users,
	})

	fileService := service.NewFileService(fs, service.FileServiceConfig{
		MountPoints: mountPoints,
	})

	searchService := service.NewSearchService(fs, service.SearchServiceConfig{
		MountPoints: mountPoints,
	})

	jobService := service.NewJobService(fs, hub, service.JobServiceConfig{
		Workers:     config.DefaultJobWorkers,
		MountPoints: mountPoints,
	})

	systemService := service.NewSystemService()

	settingsService := service.NewSettingsService(fs, service.SettingsServiceConfig{
		DataDir: cfg.DataDir,
	})

	// Create Docker service from first configured host (optional)
	var dockerService *service.DockerService
	if cfg.DockerHosts != nil {
		for id, host := range cfg.DockerHosts.Hosts {
			hostCfg := service.DockerServiceConfig{}
			switch host.Driver {
			case "tcp":
				hostCfg.Host = "tcp://" + host.Endpoint
			case "socket":
				hostCfg.SocketPath = host.Endpoint
			case "ssh":
				hostCfg.Host = "ssh://" + host.Endpoint
				hostCfg.SSHKey = host.SSHKey
			}
			svc, err := service.NewDockerService(hostCfg)
			if err != nil {
				log.Warn().Err(err).Str("host", id).Msg("Failed to create Docker service")
				continue
			}
			dockerService = svc
			break
		}
	}

	// Create handlers
	authHandler := handler.NewAuthHandler(authService)
	fileHandler := handler.NewFileHandler(fileService)
	streamHandler := handler.NewStreamHandler(fileService, cfg.ChunkSizeMB, cfg.MaxUploadMB)
	jobHandler := handler.NewJobHandler(jobService)
	searchHandler := handler.NewSearchHandler(searchService)
	wsHandler := handler.NewWebSocketHandler(hub, authService, cfg.AllowedOrigins)
	systemHandler := handler.NewSystemHandler(systemService)
	settingsHandler := handler.NewSettingsHandler(settingsService)
	var dockerHandler *handler.DockerHandler
	var sseHandler *handler.SSEHandler
	hostHandler := handler.NewHostHandler(
		func() *model.ServerConfig { return cfg },
		func(hosts *model.DockerHostsConfig) error {
			cfg.DockerHosts = hosts
			return config.Save(cfg)
		},
		func(hostID string, mps []model.MountPoint) {
			fileHandler.SetMountPoints(hostID, mps)
		},
	)
	if dockerService != nil {
		dockerHandler = handler.NewDockerHandler(dockerService, nil)
		// Create DockerService for each configured host
		if cfg.DockerHosts != nil {
			dockerHandler.SetDefaultHost(cfg.DockerHosts.Default)
			for id, host := range cfg.DockerHosts.Hosts {
				hostCfg := service.DockerServiceConfig{}
				switch host.Driver {
				case "tcp":
					hostCfg.Host = "tcp://" + host.Endpoint
				case "socket":
					hostCfg.SocketPath = host.Endpoint
				case "ssh":
					hostCfg.Host = "ssh://" + host.Endpoint
					hostCfg.SSHKey = host.SSHKey
				}
				svc, err := service.NewDockerService(hostCfg)
				if err != nil {
					log.Warn().Err(err).Str("host", id).Msg("Failed to create Docker service for host")
					continue
				}
				dockerHandler.SetService(id, svc)
				
				// Extract compose paths from host mount points: key "docker" is the main directory
				var hostComposePaths []string
				if dockerMP, ok := host.MountPoints["docker"]; ok && dockerMP != nil {
					hostComposePaths = append(hostComposePaths, dockerMP.Path)
				}
				dockerHandler.SetComposePaths(id, hostComposePaths)
			}
		}
		collector := service.NewCollector(ctx, dockerService)
		sseHandler = handler.NewSSEHandler(dockerService, collector)

		// Register per-host mount points and Docker services for file handler
		if cfg.DockerHosts != nil {
			fileHandler.SetDefaultHost(cfg.DockerHosts.Default)
			for id, host := range cfg.DockerHosts.Hosts {
				var mps []model.MountPoint
				for name, mp := range host.MountPoints {
					mps = append(mps, model.MountPoint{
						Name:     name,
						Path:     mp.Path,
						ReadOnly: mp.ReadOnly,
					})
				}
				fileHandler.SetMountPoints(id, mps)
				// Create HostFileAccess for this host
				var access service.HostFileAccess
				switch host.Driver {
				case "ssh":
					if svc, ok := dockerHandler.Services()[id]; ok {
						access = service.NewSSHFileAccess(svc)
					}
				case "tcp", "socket":
					access = service.NewSocketFileAccess()
				}
				if access != nil {
					fileHandler.SetHostAccess(id, access)
					streamHandler.SetHostAccess(id, access)
					jobHandler.SetHostAccess(id, access)
				}
				// Set host mount points for path resolution
				mounts := make(map[string]string)
				for name, mp := range host.MountPoints {
					mounts[name] = mp.Path
				}
				streamHandler.SetHostMountPoints(id, mounts)
				jobHandler.SetHostMountPoints(id, mounts)
			}
		}

		if cfg.DockerHosts != nil {
			sseHandler.SetDefaultHost(cfg.DockerHosts.Default)
			for id, host := range cfg.DockerHosts.Hosts {
				hostCfg := service.DockerServiceConfig{}
				switch host.Driver {
				case "tcp":
					hostCfg.Host = "tcp://" + host.Endpoint
				case "socket":
					hostCfg.SocketPath = host.Endpoint
				case "ssh":
					hostCfg.Host = "ssh://" + host.Endpoint
					hostCfg.SSHKey = host.SSHKey
				}
				svc, err := service.NewDockerService(hostCfg)
				if err != nil {
					continue
				}
				sseHandler.SetService(id, svc)
				var reader service.FileReader = &service.LocalFileReader{}
				if host.Driver == "ssh" {
					reader = &service.SSHFileReader{Docker: svc}
				}
				hostCollector := service.NewCollectorWithReader(ctx, svc, reader)
				sseHandler.SetCollector(id, hostCollector)
			}
		}
		_ = collector // stopped via context cancel
	}

	// Create router
	router := createRouter(cfg, authService, authHandler, fileHandler, streamHandler, jobHandler, searchHandler, wsHandler, systemHandler, settingsHandler, dockerHandler, sseHandler, hostHandler, mountPoints)

	// Create HTTP server
	server := &http.Server{
		Addr:         fmt.Sprintf("%s:%d", cfg.Host, cfg.Port),
		Handler:      router,
		ReadTimeout:  config.HTTPReadTimeout,
		WriteTimeout: config.HTTPWriteTimeout,
		IdleTimeout:  config.HTTPIdleTimeout,
	}

	return server, hub, jobService, authService, streamHandler, nil
}

// createRouter sets up chi router with all routes and middleware
func createRouter(
	cfg *model.ServerConfig,
	authService service.AuthService,
	authHandler *handler.AuthHandler,
	fileHandler *handler.FileHandler,
	streamHandler *handler.StreamHandler,
	jobHandler *handler.JobHandler,
	searchHandler *handler.SearchHandler,
	wsHandler *handler.WebSocketHandler,
	systemHandler *handler.SystemHandler,
	settingsHandler *handler.SettingsHandler,
	dockerHandler *handler.DockerHandler,
	sseHandler *handler.SSEHandler,
	hostHandler *handler.HostHandler,
	mountPoints []model.MountPoint,
) chi.Router {
	r := chi.NewRouter()

	// Global middleware
	r.Use(chimiddleware.RequestID)
	r.Use(chimiddleware.Logger)
	r.Use(chimiddleware.Recoverer)
	r.Use(middleware.SecurityHeaders)

	// Health check endpoint (no auth required)
	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"ok"}`))
	})

	// API routes
	r.Route("/api/v1", func(r chi.Router) {
		// Health check also available under API path
		r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"status":"ok"}`))
		})
		// Public routes (no auth required)
		// Auth routes are rate-limited to prevent brute force attacks
		r.Route("/auth", func(r chi.Router) {
			r.Use(middleware.RateLimit(cfg.RateLimitRPS))
			authHandler.RegisterRoutes(r)
		})

		// Protected routes (auth required)
		r.Group(func(r chi.Router) {
			r.Use(middleware.JWTAuth(authService))

			// File operations with mount point guard
			r.Route("/files", func(r chi.Router) {
				r.Use(middleware.MountPointGuard(mountPoints))
				fileHandler.RegisterRoutes(r)
			})

			// Streaming operations with mount point guard
			r.Route("/stream", func(r chi.Router) {
				r.Use(middleware.MountPointGuard(mountPoints))
				streamHandler.RegisterRoutes(r)
			})

			// Search operations
			r.Route("/search", func(r chi.Router) {
				searchHandler.RegisterRoutes(r)
			})

			// Job operations
			r.Route("/jobs", func(r chi.Router) {
				jobHandler.RegisterRoutes(r)
			})

			// System operations
			r.Route("/system", func(r chi.Router) {
				systemHandler.RegisterRoutes(r)
			})

			// Settings operations
			r.Route("/settings", func(r chi.Router) {
				settingsHandler.RegisterRoutes(r)
			})

			// Host management
			r.Route("/hosts", func(r chi.Router) {
				hostHandler.RegisterRoutes(r)
			})
			r.Get("/ssh-instructions", hostHandler.SSHKeyPairInstructions)
			r.Post("/ssh/genkey", hostHandler.SSHKeyGen)

			// Docker operations (optional)
			if dockerHandler != nil {
				r.Route("/docker", func(r chi.Router) {
					dockerHandler.RegisterRoutes(r)
				})
			}

			// SSE operations (optional)
			if sseHandler != nil {
				r.Route("/sse", func(r chi.Router) {
					sseHandler.RegisterRoutes(r)
				})
			}
		})

		// Docker exec WebSocket (auth handled in handler)
		if dockerHandler != nil {
			r.Get("/docker/containers/{id}/exec", dockerHandler.ExecWebSocket)
		}

		// WebSocket endpoint (auth handled in handler)
		r.Get("/ws", wsHandler.ServeWS)
	})

	// Static file handler for SPA frontend (catch-all)
	// This must be after all API routes
	if cfg.DevMode {
		log.Info().Msg("Dev mode: static file handler skipped, use npm run dev for frontend")
	} else {
		staticHandler, err := static.NewHandler()
		if err != nil {
			log.Warn().Err(err).Msg("Static handler not available, frontend will not be served")
		} else {
			r.NotFound(staticHandler.ServeHTTP)
			log.Info().Msg("Static file handler initialized for SPA frontend")
		}
	}

	return r
}

// waitForShutdown handles graceful shutdown on interrupt signals
func waitForShutdown(cancel context.CancelFunc, server *http.Server, jobService service.JobService, authService service.AuthService, streamHandler *handler.StreamHandler) {
	// Create channel to receive OS signals
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)

	// Wait for signal
	sig := <-sigCh
	log.Info().Str("signal", sig.String()).Msg("Received shutdown signal")

	// Cancel context to stop background goroutines
	cancel()

	// Create shutdown context with timeout
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), config.ShutdownTimeout)
	defer shutdownCancel()

	// Stop job service
	log.Info().Msg("Stopping job service...")
	jobService.Stop()

	// Shutdown HTTP server
	log.Info().Msg("Shutting down HTTP server...")
	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Error().Err(err).Msg("Error during server shutdown")
	}

	log.Info().Msg("Server shutdown complete")

	// Stop background cleanups
	authService.StopCleanup()
	streamHandler.StopCleanup()
}

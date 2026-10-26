package main

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"syscall"
	"time"

	"golang.org/x/sync/errgroup"

	"e2e-framework/internal/adapters/primary/api"
	"e2e-framework/internal/adapters/primary/cron"
	"e2e-framework/internal/adapters/primary/webhook"
	webhookproviders "e2e-framework/internal/adapters/primary/webhook/providers"
	"e2e-framework/internal/adapters/secondary/notifier"
	"e2e-framework/internal/adapters/secondary/receiver"
	receiverapi "e2e-framework/internal/adapters/secondary/receiver/api"
	"e2e-framework/internal/adapters/secondary/receiver/imap"
	receiverwebhook "e2e-framework/internal/adapters/secondary/receiver/webhook"
	"e2e-framework/internal/adapters/secondary/store"
	"e2e-framework/internal/adapters/secondary/trigger"
	"e2e-framework/internal/core/domain"
	"e2e-framework/internal/core/ports"
	"e2e-framework/internal/core/services"
	"e2e-framework/internal/pkg/assertion"
	"e2e-framework/internal/pkg/config"
	"e2e-framework/internal/pkg/logger"
)

// @title e2e-framework API
// @version 1.0
// @description This is the API for the e2e-framework testing service.
// @BasePath /
func main() {
	cfgPath := os.Getenv("CONFIG_PATH")
	if cfgPath == "" {
		cfgPath = "configs/config.yaml"
	}

	// Bootstrap logger (info/json) until the config block is loaded; every
	// package builds its own logger with logger.New("<component>") after
	// logger.Configure has been called once here.
	lg := logger.New("main")

	cfg, err := config.LoadConfig(cfgPath)
	if err != nil {
		lg.Fatal().
			Str("config_path", cfgPath).
			Err(err).
			Msg("failed to load configuration")
	}

	if err := logger.Configure(cfg.Logging.Level, cfg.Logging.Format); err != nil {
		lg.Fatal().
			Err(err).
			Msg("failed to configure logging")
	}

	lg = logger.New("main")

	lg.Info().
		Str("config_path", cfgPath).
		Str("log_level", cfg.Logging.Level).
		Str("log_format", cfg.Logging.Format).
		Msg("service starting")

	testsPath := cfg.Tests.Path
	if testsPath == "" {
		testsPath = "tests"
	}

	tests, err := config.LoadTestDefinitions(testsPath)
	if err != nil {
		lg.Fatal().
			Str("tests_path", testsPath).
			Err(err).
			Msg("failed to load test definitions")
	}

	lg.Info().
		Int("tests_count", len(tests)).
		Msg("test definitions loaded")

	storeRegistry := store.NewStoreRegistry()
	storeRegistry.Register("redis", func(cfg config.StoreConfig) (ports.Store, error) {
		s, err := store.NewRedisStore(cfg.Redis)
		if err != nil {
			return nil, err
		}

		lg.Info().
			Str(logger.AttrStoreType, "redis").
			Str(logger.AttrHost, hostFromURL(cfg.Redis.URL)).
			Msg("store connection established")

		return s, nil
	})
	storeRegistry.Register("postgres", func(cfg config.StoreConfig) (ports.Store, error) {
		s, err := store.NewPostgresStore(cfg.Postgres)
		if err != nil {
			return nil, err
		}

		lg.Info().
			Str(logger.AttrStoreType, "postgres").
			Msg("store connection established")

		return s, nil
	})
	storeRegistry.Register("memory", func(cfg config.StoreConfig) (ports.Store, error) {
		lg.Info().
			Str(logger.AttrStoreType, "memory").
			Msg("store connection established")

		return store.NewMemoryStore(cfg.Memory), nil
	})
	storeRegistry.Register("disabled", func(cfg config.StoreConfig) (ports.Store, error) {
		lg.Info().
			Str(logger.AttrStoreType, "disabled").
			Msg("store connection established")

		return store.NewDisabledStore(), nil
	})

	s, err := storeRegistry.Create(cfg.Store)
	if err != nil {
		lg.Fatal().
			Str(logger.AttrStoreType, cfg.Store.Type).
			Err(err).
			Msg("failed to create store")
	}

	defer func() {
		if err := s.Close(); err != nil {
			lg.Warn().
				Str(logger.AttrStoreType, cfg.Store.Type).
				Err(err).
				Msg("failed to close store")

			return
		}

		lg.Debug().
			Str(logger.AttrStoreType, cfg.Store.Type).
			Msg("store closed")
	}()

	assertionRegistry := assertion.NewDefaultRegistry()
	httpClient := &http.Client{Timeout: 30 * time.Second}

	triggerRegistry := trigger.NewTriggerRegistry()
	triggerRegistry.Register(domain.HTTPTriggerType, func(options map[string]string) (ports.Trigger, error) {
		return trigger.NewHTTPTrigger(assertionRegistry, httpClient)
	})

	httpNotifier := notifier.NewHTTPNotifier()

	receiverRegistry := receiver.NewReceiverRegistry()
	receiverRegistry.Register(
		domain.WebhookReceiverType,
		func(cfg domain.ReceiverConfig) (ports.Receiver, error) {
			return receiverwebhook.NewWebhookReceiver(s), nil
		},
	)
	receiverRegistry.Register(
		domain.ImapReceiverType,
		func(cfg domain.ReceiverConfig) (ports.Receiver, error) {
			return imap.NewIMAPReceiver(cfg.Options)
		},
	)
	receiverRegistry.Register(
		domain.APIReceiverType,
		func(cfg domain.ReceiverConfig) (ports.Receiver, error) {
			return receiverapi.NewAPIPollingReceiver(cfg, assertionRegistry, httpClient)
		},
	)

	// Core Orchestrator
	orchestrator := services.NewOrchestrator(
		triggerRegistry,
		s,
		receiverRegistry,
		assertionRegistry,
		httpNotifier,
	)

	// Setup primary adapters
	if err := config.ValidateTestGroups(cfg.TestGroups, tests); err != nil {
		lg.Fatal().
			Err(err).
			Msg("invalid test groups configuration")
	}

	groupResolver := services.NewGroupResolver(cfg.TestGroups)

	apiServer := api.NewServer(&api.Config{
		Port:       cfg.Server.Port,
		AuthEnable: cfg.Auth.Enabled,
		JWTSecret:  cfg.Auth.JWTSecret,
		Resolver:   groupResolver,
	}, orchestrator, tests)

	ingestor, err := services.NewIngestor(s)
	if err != nil {
		lg.Fatal().
			Err(err).
			Msg("failed to create ingestor")
	}

	whServer, err := webhook.NewServer(ingestor)
	if err != nil {
		lg.Fatal().
			Err(err).
			Msg("failed to create webhook server")
	}
	whServer.RegisterExtractor("twilio", webhookproviders.NewTwilioExtractor())
	whServer.RegisterExtractor("meta", webhookproviders.NewMetaExtractor())
	whServer.RegisterExtractor("generic", webhookproviders.NewGenericExtractor())

	whServer.RegisterRoutes(apiServer.Mux())

	scheduler := cron.NewScheduler(orchestrator)
	for _, t := range tests {
		if err := scheduler.RegisterTest(t); err != nil {
			lg.Warn().
				Str(logger.AttrTestID, t.ID).
				Err(err).
				Msg("failed to schedule test")
		}
	}

	// Main execution context
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	g, gCtx := errgroup.WithContext(ctx)

	// Start server (API + Webhook routes unified on single port)
	lg.Info().Int("port", cfg.Server.Port).Msg("http server starting")
	g.Go(func() error { return apiServer.Start() })

	lg.Info().Msg("scheduler starting")
	scheduler.Start()

	// Shutdown handlers
	g.Go(func() error {
		<-gCtx.Done()
		lg.Info().Msg("http server stopping")
		return apiServer.Stop()
	})

	g.Go(func() error {
		<-gCtx.Done()
		lg.Info().Msg("scheduler stopping")
		scheduler.Stop()
		return nil
	})

	lg.Info().Msg("service is running. press ctrl-c to stop.")

	if err := g.Wait(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		lg.Error().Err(err).Msg("service stopped with error")
	} else {
		lg.Info().Msg("service stopped cleanly")
	}
}

// hostFromURL returns only the host part of a connection URL so credentials
// and query parameters never reach the logs.
func hostFromURL(raw string) string {
	if raw == "" {
		return ""
	}

	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return ""
	}

	if _, _, err := net.SplitHostPort(u.Host); err == nil {
		return u.Host
	}

	return u.Hostname()
}

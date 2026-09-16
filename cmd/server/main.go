package main

import (
	"context"
	"log"
	"net/http"
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
)

// @title e2e-framework API
// @version 1.0
// @description This is the API for the e2e-framework testing service.
// @BasePath /
func main() {
	log.Println("Starting e2e-testing-service...")

	cfgPath := os.Getenv("CONFIG_PATH")
	if cfgPath == "" {
		cfgPath = "configs/config.yaml"
	}

	cfg, err := config.LoadConfig(cfgPath)
	if err != nil {
		log.Fatalf("failed to load config: %v", err)
	}

	testsPath := cfg.Tests.Path
	if testsPath == "" {
		testsPath = "tests"
	}

	tests, err := config.LoadTestDefinitions(testsPath)
	if err != nil {
		log.Fatalf("failed to load tests: %v", err)
	}

	log.Printf("Loaded %d test definitions", len(tests))

	storeRegistry := store.NewStoreRegistry()
	storeRegistry.Register("redis", func(cfg config.StoreConfig) (ports.Store, error) {
		return store.NewRedisStore(cfg.Redis)
	})
	storeRegistry.Register("postgres", func(cfg config.StoreConfig) (ports.Store, error) {
		return store.NewPostgresStore(cfg.Postgres)
	})
	storeRegistry.Register("memory", func(cfg config.StoreConfig) (ports.Store, error) {
		return store.NewMemoryStore(cfg.Memory), nil
	})
	storeRegistry.Register("disabled", func(cfg config.StoreConfig) (ports.Store, error) {
		return store.NewDisabledStore(), nil
	})

	s, err := storeRegistry.Create(cfg.Store)
	if err != nil {
		log.Fatalf("failed to create store: %v", err)
	}
	defer s.Close()
	log.Printf("Store initialized with type: %s", cfg.Store.Type)

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
		log.Fatalf("invalid test groups config: %v", err)
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
		log.Fatalf("failed to create ingestor: %v", err)
	}

	whServer, err := webhook.NewServer(ingestor)
	if err != nil {
		log.Fatalf("failed to create webhook server: %v", err)
	}
	whServer.RegisterExtractor("twilio", webhookproviders.NewTwilioExtractor())
	whServer.RegisterExtractor("meta", webhookproviders.NewMetaExtractor())
	whServer.RegisterExtractor("generic", webhookproviders.NewGenericExtractor())

	whServer.RegisterRoutes(apiServer.Mux())

	scheduler := cron.NewScheduler(orchestrator)
	for _, t := range tests {
		if err := scheduler.RegisterTest(t); err != nil {
			log.Printf("Warning: failed to schedule test %s: %v", t.ID, err)
		}
	}

	// Main execution context
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	g, gCtx := errgroup.WithContext(ctx)

	// Start server (API + Webhook routes unified on single port)
	log.Printf("Starting server on port %d", cfg.Server.Port)
	g.Go(func() error { return apiServer.Start() })

	log.Println("Starting Cron scheduler")
	scheduler.Start()

	// Shutdown handlers
	g.Go(func() error {
		<-gCtx.Done()
		log.Println("Shutting down server...")
		return apiServer.Stop()
	})

	g.Go(func() error {
		<-gCtx.Done()
		log.Println("Shutting down Cron scheduler...")
		scheduler.Stop()
		return nil
	})

	log.Println("Service is running. Press CTRL-C to stop.")
	if err := g.Wait(); err != nil {
		log.Printf("Service stopped with error: %v", err)
	} else {
		log.Println("Service stopped cleanly.")
	}
}

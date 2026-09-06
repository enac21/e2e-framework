package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"golang.org/x/sync/errgroup"

	"e2e-framework/internal/adapters/primary/api"
	"e2e-framework/internal/adapters/primary/cron"
	"e2e-framework/internal/adapters/primary/webhook"
	receiverasserts "e2e-framework/internal/adapters/secondary/assertions/receiver"
	triggerasserts "e2e-framework/internal/adapters/secondary/assertions/trigger"
	"e2e-framework/internal/adapters/secondary/notifier"
	"e2e-framework/internal/adapters/secondary/receiver"
	"e2e-framework/internal/adapters/secondary/receiver/imap"
	"e2e-framework/internal/adapters/secondary/receiver/request"
	"e2e-framework/internal/adapters/secondary/store"
	"e2e-framework/internal/adapters/secondary/trigger"
	"e2e-framework/internal/core/domain"
	"e2e-framework/internal/core/ports"
	"e2e-framework/internal/core/services"
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

	storeReg := store.NewStoreRegistry()
	storeReg.Register("redis", func(cfg config.StoreConfig) (ports.Store, error) {
		return store.NewRedisStore(cfg.Redis)
	})
	storeReg.Register("postgres", func(cfg config.StoreConfig) (ports.Store, error) {
		return store.NewPostgresStore(cfg.Postgres)
	})
	storeReg.Register("memory", func(cfg config.StoreConfig) (ports.Store, error) {
		return store.NewMemoryStore(cfg.Memory), nil
	})
	storeReg.Register("disabled", func(cfg config.StoreConfig) (ports.Store, error) {
		return store.NewDisabledStore(), nil
	})

	s, err := storeReg.Create(cfg.Store)
	if err != nil {
		log.Fatalf("failed to create store: %v", err)
	}
	defer s.Close()

	triggerAssertionReg := triggerasserts.NewTriggerAssertionRegistry()
	triggerAssertionReg.Register("equals", triggerasserts.NewEqualsAssertion)
	triggerAssertionReg.Register("contains", triggerasserts.NewContainsAssertion)
	triggerAssertionReg.Register("not_contains", triggerasserts.NewNotContainsAssertion)
	triggerAssertionReg.Register("present", triggerasserts.NewPresentAssertion)
	triggerAssertionReg.Register("matches", triggerasserts.NewMatchesAssertion)
	triggerAssertionReg.Register("array_contains", triggerasserts.NewArrayContainsAssertion)
	triggerAssertionReg.Register("map_contains", triggerasserts.NewMapContainsAssertion)
	triggerAssertionReg.Register("length", triggerasserts.NewLengthAssertion)
	triggerAssertionReg.Register("int_eq", triggerasserts.NewIntEqAssertion)
	triggerAssertionReg.Register("int_gt", triggerasserts.NewIntGtAssertion)
	triggerAssertionReg.Register("int_gte", triggerasserts.NewIntGteAssertion)
	triggerAssertionReg.Register("int_lt", triggerasserts.NewIntLtAssertion)
	triggerAssertionReg.Register("int_lte", triggerasserts.NewIntLteAssertion)

	triggerReg := trigger.NewTriggerRegistry()
	triggerReg.Register(domain.HTTPTriggerType, func(options map[string]string) (ports.Trigger, error) {
		return trigger.NewHTTPTrigger(triggerAssertionReg), nil
	})

	httpNotifier := notifier.NewHTTPNotifier()

	assertionReg := receiverasserts.NewReceiverAssertionRegistry()
	assertionReg.Register("contains", receiverasserts.NewContainsAssertion)
	assertionReg.Register("equals", receiverasserts.NewEqualsAssertion)
	assertionReg.Register("matches", receiverasserts.NewMatchesAssertion)
	assertionReg.Register("present", receiverasserts.NewPresentAssertion)
	assertionReg.Register("not_contains", receiverasserts.NewNotContainsAssertion)

	receiverReg := receiver.NewReceiverRegistry()
	receiverReg.Register(
		domain.RequestReceiverType,
		func(options map[string]string) (ports.Receiver, error) {
			return request.NewRequestReceiver(s), nil
		},
	)
	receiverReg.Register(
		domain.ImapReceiverType,
		func(options map[string]string) (ports.Receiver, error) {
			return imap.NewIMAPReceiver(options)
		},
	)

	// Core Orchestrator
	orchestrator := services.NewOrchestrator(
		triggerReg,
		s,
		receiverReg,
		assertionReg,
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

	whServer := webhook.NewServer(s)
	whServer.RegisterExtractor("twilio", webhook.NewTwilioExtractor())
	whServer.RegisterExtractor("meta", webhook.NewMetaExtractor())
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

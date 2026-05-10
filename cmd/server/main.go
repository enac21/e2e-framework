package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"golang.org/x/sync/errgroup"

	"e2e-framework/internal/adapters/primary/cron"
	api "e2e-framework/internal/adapters/primary/http"
	"e2e-framework/internal/adapters/primary/webhook"
	"e2e-framework/internal/adapters/secondary/assertion"
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

	cfg, err := config.LoadConfig("configs/config.yaml")
	if err != nil {
		log.Fatalf("failed to load config: %v", err)
	}

	tests, err := config.LoadTestDefinitions("tests")
	if err != nil {
		log.Fatalf("failed to load tests: %v", err)
	}

	log.Printf("Loaded %d test definitions", len(tests))

	// Setup secondary adapters
	redisStore, err := store.NewRedisStore(store.RedisStoreConfig{
		URL: cfg.Store.Redis.URL,
		TTL: 24 * time.Hour,
	})
	if err != nil {
		log.Fatalf("failed to connect to store: %v", err)
	}
	defer redisStore.Close()

	httpTrigger := trigger.NewHTTPTrigger()
	webhookNotifier := notifier.NewWebhookNotifier()

	assertionReg := assertion.NewAssertionRegistry()
	assertionReg.Register("contains", assertion.NewContainsAssertion)
	assertionReg.Register("equals", assertion.NewEqualsAssertion)
	assertionReg.Register("matches", assertion.NewMatchesAssertion)
	assertionReg.Register("present", assertion.NewPresentAssertion)
	assertionReg.Register("not_contains", assertion.NewNotContainsAssertion)

	receiverReg := receiver.NewReceiverRegistry()
	receiverReg.Register(
		domain.RequestReceiverType,
		func(options map[string]string) (ports.Receiver, error) {
			return request.NewRequestReceiver(redisStore), nil
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
		httpTrigger,
		redisStore,
		receiverReg,
		assertionReg,
		webhookNotifier,
	)

	// Setup primary adapters
	apiServer := api.NewServer(&api.Config{
		Port:       cfg.Server.Port,
		AuthEnable: cfg.Auth.Enabled,
		JWTSecret:  cfg.Auth.JWTSecret,
	}, orchestrator, tests)

	whServer := webhook.NewServer(&webhook.Config{
		Port: cfg.Webhook.Port,
		//TODO - Add auth config
	}, redisStore)
	whServer.RegisterExtractor("twilio", webhook.NewTwilioExtractor())
	whServer.RegisterExtractor("meta", webhook.NewMetaExtractor())

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

	// Start servers
	log.Printf("Starting API server on port %d", cfg.Server.Port)
	g.Go(func() error { return apiServer.Start() })

	log.Printf("Starting Webhook server on port %d", cfg.Webhook.Port)
	g.Go(func() error { return whServer.Start() })

	log.Println("Starting Cron scheduler")
	scheduler.Start()

	// Shutdown handlers
	g.Go(func() error {
		<-gCtx.Done()
		log.Println("Shutting down API server...")
		return apiServer.Stop()
	})

	g.Go(func() error {
		<-gCtx.Done()
		log.Println("Shutting down Webhook server...")
		return whServer.Stop()
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

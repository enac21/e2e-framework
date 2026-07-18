package main

import (
	"log"

	"e2e-framework/internal/adapters/primary/api"
	"e2e-framework/internal/adapters/primary/cron"
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

type App struct {
	apiServer  *api.Server
	scheduler  *cron.Scheduler
	redisStore *store.RedisStore
}

func Setup(cfg *config.Config, tests map[string]domain.TestDefinition) (*App, error) {
	redisStore, err := store.NewRedisStore(store.RedisStoreConfig{
		URL:         cfg.Store.Redis.URL,
		Username:    cfg.Store.Redis.Username,
		Password:    cfg.Store.Redis.Password,
		ClusterMode: cfg.Store.Redis.ClusterMode,
		TTL:         cfg.Store.Redis.TTL,
	})
	if err != nil {
		return nil, err
	}

	httpTrigger := trigger.NewHTTPTrigger()
	webhookNotifier := notifier.NewWebhookNotifier()

	assertionReg := assertion.NewAssertionRegistry().
		Register("contains", assertion.NewContainsAssertion).
		Register("equals", assertion.NewEqualsAssertion).
		Register("matches", assertion.NewMatchesAssertion).
		Register("present", assertion.NewPresentAssertion).
		Register("not_contains", assertion.NewNotContainsAssertion)

	//TODO - Handle the creation of the receivers based on the config. If not configured, ignore it
	receiverReg := receiver.NewReceiverRegistry().
		Register(
			domain.RequestReceiverType,
			func(options map[string]string) (ports.Receiver, error) {
				return request.NewRequestReceiver(redisStore), nil
			},
		).
		Register(
			domain.ImapReceiverType,
			func(options map[string]string) (ports.Receiver, error) {
				return imap.NewIMAPReceiver(options)
			},
		)

	orchestrator := services.NewOrchestrator(
		httpTrigger,
		redisStore,
		receiverReg,
		assertionReg,
		webhookNotifier,
	)

	apiServer := api.NewServer(
		&api.Config{
			Port:       cfg.Server.Port,
			AuthEnable: cfg.Auth.Enabled,
			JWTSecret:  cfg.Auth.JWTSecret,
		},
		orchestrator,
		tests,
	)

	webhook.NewServer(redisStore).
		RegisterExtractor("twilio", webhook.NewTwilioExtractor()).
		RegisterExtractor("meta", webhook.NewMetaExtractor()).
		RegisterRoutes(apiServer.Mux())

	scheduler := cron.NewScheduler(orchestrator)
	for _, t := range tests {
		if err := scheduler.RegisterTest(t); err != nil {
			log.Printf("Warning: failed to schedule test %s: %v", t.ID, err)
		}
	}

	return &App{
		apiServer:  apiServer,
		scheduler:  scheduler,
		redisStore: redisStore,
	}, nil
}

func (a *App) Start() error {
	go func() {
		if err := a.apiServer.Start(); err != nil {
			log.Printf("API server error: %v", err)
		}
	}()

	log.Println("Starting Cron scheduler")
	a.scheduler.Start()

	return nil
}

func (a *App) Stop() {
	log.Println("Shutting down Cron scheduler...")
	a.scheduler.Stop()

	log.Println("Shutting down server...")
	if err := a.apiServer.Stop(); err != nil {
		log.Printf("API server stop error: %v", err)
	}

	log.Println("Closing Redis store...")
	if err := a.redisStore.Close(); err != nil {
		log.Printf("Redis store close error: %v", err)
	}
}

package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

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

	app, err := Setup(cfg, tests)
	if err != nil {
		log.Fatalf("failed to initialize application: %v", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := app.Start(); err != nil {
		log.Fatalf("failed to start application: %v", err)
	}

	log.Println("Service is running. Press CTRL-C to stop.")
	<-ctx.Done()

	app.Stop()
	log.Println("Service stopped cleanly.")
}

package cron

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/robfig/cron/v3"

	"e2e-framework/internal/core/domain"
	"e2e-framework/internal/core/services"
)

type Scheduler struct {
	cron         *cron.Cron
	orchestrator *services.Orchestrator
	logger       *slog.Logger
}

func NewScheduler(orchestrator *services.Orchestrator) *Scheduler {
	return &Scheduler{
		cron:         cron.New(cron.WithParser(cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow))),
		orchestrator: orchestrator,
		logger: slog.With(
			"component", "scheduler",
			"package", "cron",
		),
	}
}

func (s *Scheduler) RegisterTest(def domain.TestDefinition) error {
	if !def.Enabled || def.Schedule == "" {
		s.logger.Debug("test scheduling skipped",
			"test_id", def.ID,
			"reason", fmt.Sprintf("enabled=%v, schedule=%q", def.Enabled, def.Schedule),
		)
		return nil
	}

	_, err := s.cron.AddFunc(def.Schedule, func() {
		_, resultCh := s.orchestrator.RunTest(context.Background(), def)
		<-resultCh
	})

	if err != nil {
		s.logger.Warn("failed to schedule test",
			"test_id", def.ID,
			"schedule", def.Schedule,
			"error", err,
		)
		return fmt.Errorf("%w: failed to schedule test %s: %v", domain.ErrConfiguration, def.ID, err)
	}

	s.logger.Info("test scheduled",
		"test_id", def.ID,
		"schedule", def.Schedule,
	)

	return nil
}

func (s *Scheduler) Start() {
	s.cron.Start()
	s.logger.Info("scheduler started")
}

func (s *Scheduler) Stop() {
	s.cron.Stop()
	s.logger.Info("scheduler stopped")
}

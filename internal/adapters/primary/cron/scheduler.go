package cron

import (
	"context"
	"fmt"

	"github.com/robfig/cron/v3"

	"e2e-framework/internal/core/domain"
	"e2e-framework/internal/core/services"
)

type Scheduler struct {
	cron         *cron.Cron
	orchestrator *services.Orchestrator
}

func NewScheduler(orchestrator *services.Orchestrator) *Scheduler {
	// Use standard cron parsing (minute, hour, dom, month, dow)
	return &Scheduler{
		cron:         cron.New(cron.WithParser(cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow))),
		orchestrator: orchestrator,
	}
}

func (s *Scheduler) RegisterTest(def domain.TestDefinition) error {
	if !def.Enabled || def.Schedule == "" {
		return nil
	}

	_, err := s.cron.AddFunc(def.Schedule, func() {
		_, resultCh := s.orchestrator.RunTest(context.Background(), def)
		<-resultCh
	})

	if err != nil {
		return fmt.Errorf("failed to schedule test %s: %w", def.ID, err)
	}

	return nil
}

func (s *Scheduler) Start() {
	s.cron.Start()
}

func (s *Scheduler) Stop() {
	s.cron.Stop()
}

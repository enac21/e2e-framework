package cron

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/robfig/cron/v3"

	"e2e-framework/internal/core/domain"
	"e2e-framework/internal/core/services"
	"e2e-framework/internal/pkg/logger"
)

type Scheduler struct {
	cron         *cron.Cron
	orchestrator *services.Orchestrator
	logger       *logger.Logger
}

func NewScheduler(orchestrator *services.Orchestrator) *Scheduler {
	return &Scheduler{
		cron:         cron.New(cron.WithParser(cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow))),
		orchestrator: orchestrator,
		logger:       logger.New("cron"),
	}
}

func (s *Scheduler) RegisterTest(def domain.TestDefinition) error {
	if !def.Enabled || def.Schedule == "" {
		s.logger.Debug().
			Str(logger.AttrTestID, def.ID).
			Bool(logger.IsEnabled, def.Enabled).
			Str(logger.AttrSchedule, def.Schedule).
			Str(logger.AttrReason, "test disabled or missing schedule").
			Msg("test scheduling skipped")

		return nil
	}

	_, err := s.cron.AddFunc(def.Schedule, func() {
		s.logger.Info().
			Str(logger.AttrTestID, def.ID).
			Str(logger.AttrSchedule, def.Schedule).
			Msg("cron triggered test run")

		_, resultCh := s.orchestrator.RunTest(context.Background(), def, logger.CronFlow)
		result := <-resultCh

		level := slog.LevelInfo
		if result.Status == domain.StatusFailed || result.Status == domain.StatusError {
			level = slog.LevelError
		} else if result.Status == domain.StatusSkipped {
			level = slog.LevelWarn
		}

		e := s.logger.Event(level).
			Str(logger.AttrRunID, result.RunID).
			Str(logger.AttrTestID, result.TestID).
			Str(logger.AttrSchedule, def.Schedule).
			Str(logger.AttrStatus, string(result.Status)).
			Int(logger.AttrDurationMS, int(result.DurationMs))

		if result.Error != "" {
			e.Str(logger.AttrError, result.Error)
		}

		e.Msg("scheduled test run finished")
	})

	if err != nil {
		s.logger.Warn().
			Str(logger.AttrTestID, def.ID).
			Str(logger.AttrSchedule, def.Schedule).
			Err(err).
			Msg("failed to schedule test")

		return fmt.Errorf("%w: failed to schedule test %s: %v", domain.ErrConfiguration, def.ID, err)
	}

	s.logger.Info().
		Str(logger.AttrTestID, def.ID).
		Str(logger.AttrSchedule, def.Schedule).
		Msg("test scheduled")

	return nil
}

func (s *Scheduler) Start() {
	s.cron.Start()
	s.logger.Info().Msg("scheduler started")
}

func (s *Scheduler) Stop() {
	s.cron.Stop()
	s.logger.Info().Msg("scheduler stopped")
}

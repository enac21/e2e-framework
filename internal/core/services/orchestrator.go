package services

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"e2e-framework/internal/adapters/secondary/receiver"
	"e2e-framework/internal/adapters/secondary/trigger"
	"e2e-framework/internal/core/domain"
	"e2e-framework/internal/core/ports"
	"e2e-framework/internal/pkg/assertion"
	"e2e-framework/internal/pkg/logger"
	"e2e-framework/internal/pkg/template"
)

var reservedVarNames = map[string]bool{
	"run_id":  true,
	"test_id": true,
	"error":   true,
}

type activeReceiver struct {
	cfg      domain.ReceiverConfig
	instance ports.Receiver
}

type Orchestrator struct {
	triggers   *trigger.TriggerRegistry
	store      ports.Store
	receivers  *receiver.ReceiverRegistry
	assertions *assertion.Registry
	notifier   ports.Notifier
	logger     *logger.Logger
}

func NewOrchestrator(
	triggers *trigger.TriggerRegistry,
	store ports.Store,
	receivers *receiver.ReceiverRegistry,
	assertions *assertion.Registry,
	notifier ports.Notifier,
) *Orchestrator {
	return &Orchestrator{
		triggers:   triggers,
		store:      store,
		receivers:  receivers,
		assertions: assertions,
		notifier:   notifier,
		logger:     logger.New("orchestrator"),
	}
}

type SequenceConfig struct {
	Delay        time.Duration
	SkipFailTest bool
}

func (o *Orchestrator) RunSequence(
	ctx context.Context,
	defs []domain.TestDefinition,
	cfg SequenceConfig,
) []*domain.TestResult {
	seqStart := time.Now()

	o.logger.Info().
		Str(logger.AttrSource, logger.SequenceFlow).
		Int(logger.AttrTestsCount, len(defs)).
		Bool("skip_fail_test", cfg.SkipFailTest).
		Msg("Test sequence started")

	results := make([]*domain.TestResult, 0, len(defs))
	failedCount := 0

	for i, def := range defs {
		if i > 0 && cfg.Delay > 0 {
			time.Sleep(cfg.Delay)
		}

		_, resultCh := o.RunTest(ctx, def, logger.SequenceFlow)
		result := <-resultCh
		results = append(results, result)

		if cfg.SkipFailTest && (result.Status == domain.StatusFailed || result.Status == domain.StatusError) {
			failedCount++

			break
		}
	}

	o.logger.Info().
		Str(logger.AttrSource, logger.SequenceFlow).
		Int(logger.AttrTestsCount, len(results)).
		Int(logger.AttrFailedCount, failedCount).
		Duration(logger.AttrDurationMS, time.Since(seqStart)).
		Msg("Test sequence finished")

	return results
}

func (o *Orchestrator) RunTest(ctx context.Context, def domain.TestDefinition, source string) (string, <-chan *domain.TestResult) {
	runID := fmt.Sprintf("%s-%d", def.ID, time.Now().UnixNano())
	resultCh := make(chan *domain.TestResult, 1)

	go func() {
		resultCh <- o.execute(ctx, def, runID, source)
	}()

	return runID, resultCh
}

func (o *Orchestrator) execute(ctx context.Context, def domain.TestDefinition, runID string, source string) *domain.TestResult {
	startTime := time.Now()

	result := &domain.TestResult{
		TestID:    def.ID,
		RunID:     runID,
		Status:    domain.StatusPassed,
		StartedAt: startTime,
	}

	if !def.Enabled {
		result.Status = domain.StatusSkipped
		result.FinishedAt = time.Now()
		result.DurationMs = result.FinishedAt.Sub(startTime).Milliseconds()

		o.logRunFinished(result, source)

		return result
	}

	o.executeSequential(ctx, def, runID, result)

	result.FinishedAt = time.Now()
	result.DurationMs = result.FinishedAt.Sub(startTime).Milliseconds()

	if result.Status == domain.StatusFailed || result.Status == domain.StatusError {
		go o.notifyFailure(context.Background(), def.OnFailure, result)
	}

	o.logRunFinished(result, source)

	return result
}

func (o *Orchestrator) logRunFinished(result *domain.TestResult, source string) {
	e := o.logger.Event(runStatusLevel(result.Status)).
		Str(logger.AttrRunID, result.RunID).
		Str(logger.AttrTestID, result.TestID).
		Str(logger.AttrSource, source).
		Str(logger.AttrStatus, string(result.Status)).
		Int(logger.AttrDurationMS, int(result.DurationMs)).
		Int("attempts", result.Attempts)

	if result.Error != "" {
		e.Str(logger.AttrError, result.Error)
	}

	e.Msg("Test run finished")
}

func runStatusLevel(status domain.RunStatus) slog.Level {
	switch status {
	case domain.StatusFailed, domain.StatusError:
		return slog.LevelError
	case domain.StatusSkipped:
		return slog.LevelWarn
	default:
		return slog.LevelInfo
	}
}

func (o *Orchestrator) executeSequential(
	ctx context.Context,
	def domain.TestDefinition,
	runID string,
	result *domain.TestResult,
) {
	triggerVars := make(map[string]string)
	triggerVars["run_id"] = runID

	for k, v := range def.Variables {
		if reservedVarNames[k] {
			o.logger.Warn().
				Str(logger.AttrRunID, runID).
				Str(logger.AttrTestID, def.ID).
				Str(logger.AttrName, k).
				Msg("Reserved variable ignored")

			continue
		}

		triggerVars[k] = template.ReplaceString(v, triggerVars)
	}

	for i, triggerStep := range def.Triggers {
		step := i + 1

		o.logger.Debug().
			Str(logger.AttrRunID, runID).
			Str(logger.AttrTestID, def.ID).
			Int(logger.AttrStep, step).
			Str(logger.AttrTriggerType, triggerStep.EffectiveType()).
			Msg("Step started")

		if triggerStep.DelayBefore > 0 {
			o.logger.Debug().
				Str(logger.AttrRunID, runID).
				Str(logger.AttrTestID, def.ID).
				Int(logger.AttrStep, step).
				Duration(logger.AttrDelayMS, triggerStep.DelayBefore).
				Msg("Waiting before step execution")

			time.Sleep(triggerStep.DelayBefore)
		}

		stepTrigger, err := o.triggers.Create(triggerStep.EffectiveType(), triggerStep.Options)
		if err != nil {
			result.TriggerVars = triggerVars
			o.failResult(result, err.Error())

			return
		}

		reserved, err := o.reserveRecipients(ctx, triggerStep.Receivers, runID)
		if err != nil {
			result.TriggerVars = triggerVars
			o.failResult(result, err.Error())

			return
		}

		defer o.releaseRecipients(ctx, reserved)

		stepMaxAttempts := 1
		if def.Retry.Enabled && def.Retry.Attempts > 1 {
			stepMaxAttempts = def.Retry.Attempts
		}

		stepPassed := false

		var lastTriggerErr error

		stepStart := time.Now()

		for attempt := 1; attempt <= stepMaxAttempts; attempt++ {
			result.Attempts++

			stepVars, triggerErr := stepTrigger.Execute(ctx, triggerStep, runID, triggerVars)
			if triggerErr != nil {
				lastTriggerErr = triggerErr

				logEvent := o.logger.Warn
				if attempt == stepMaxAttempts {
					logEvent = o.logger.Error
				}

				logEvent().
					Str(logger.AttrRunID, runID).
					Str(logger.AttrTestID, def.ID).
					Int(logger.AttrStep, step).
					Int(logger.AttrAttempt, attempt).
					Int(logger.AttrMaxAttempts, stepMaxAttempts).
					Err(triggerErr).
					Msg("Step attempt failed")

				if attempt < stepMaxAttempts {
					time.Sleep(def.Retry.Delay)
				}

				continue
			}

			if stepVars == nil {
				stepVars = make(map[string]string)
			}

			for k, v := range stepVars {
				triggerVars[k] = v
			}

			if !triggerStep.WaitForReceivers || len(triggerStep.Receivers) == 0 {
				stepPassed = true

				break
			}

			active, startErr := o.startReceivers(ctx, triggerStep.Receivers, runID, triggerVars)
			if startErr != nil {
				o.failResult(result, fmt.Sprintf("step %d receiver start failed: %v", i+1, startErr))

				break
			}

			o.collectAndAssertAll(ctx, runID, active, triggerVars, result, i)
			o.stopReceivers(active)

			if result.Status == domain.StatusPassed {
				stepPassed = true

				break
			}

			if attempt < stepMaxAttempts {
				time.Sleep(def.Retry.Delay)
			}
		}

		result.TriggerVars = triggerVars

		if !stepPassed {
			if lastTriggerErr != nil {
				o.failResult(result, fmt.Sprintf("step %d trigger failed: %v", i+1, lastTriggerErr))
			}

			return
		}

		o.logger.Debug().
			Str(logger.AttrRunID, runID).
			Str(logger.AttrTestID, def.ID).
			Int(logger.AttrStep, step).
			Duration(logger.AttrDurationMS, time.Since(stepStart)).
			Msg("Step completed")

		result.Status = domain.StatusPassed
	}

	result.TriggerVars = triggerVars
}

func (o *Orchestrator) reserveRecipients(ctx context.Context, configs []domain.ReceiverConfig, runID string) ([]domain.ReceiverConfig, error) {
	reserved := make([]domain.ReceiverConfig, 0, len(configs))

	for _, rcfg := range configs {
		if rcfg.Recipient == "" {
			continue
		}

		if err := o.store.Reserve(ctx, rcfg.Type, rcfg.Recipient, runID); err != nil {
			return reserved, fmt.Errorf("recipient reservation failed for %s: %w", rcfg.Type, err)
		}

		reserved = append(reserved, rcfg)
	}

	return reserved, nil
}

func (o *Orchestrator) releaseRecipients(ctx context.Context, reserved []domain.ReceiverConfig) {
	for _, rcfg := range reserved {
		_ = o.store.Release(ctx, rcfg.Type, rcfg.Recipient)
	}
}

func (o *Orchestrator) startReceivers(ctx context.Context, configs []domain.ReceiverConfig, runID string, triggerVars map[string]string) ([]activeReceiver, error) {
	active := make([]activeReceiver, 0, len(configs))

	for _, rcfg := range configs {
		instance, err := o.receivers.Create(rcfg)
		if err != nil {
			o.logger.Warn().
				Str(logger.AttrRunID, runID).
				Str(logger.AttrReceiver, rcfg.Type).
				Err(err).
				Msg("Failed to start receiver")

			return active, fmt.Errorf("failed to create receiver %s: %w", rcfg.Type, err)
		}

		if err := instance.Start(ctx, runID, triggerVars); err != nil {
			o.logger.Warn().
				Str(logger.AttrRunID, runID).
				Str(logger.AttrReceiver, rcfg.Type).
				Err(err).
				Msg("Failed to start receiver")

			return active, fmt.Errorf("failed to start receiver %s: %w", rcfg.Type, err)
		}

		o.logger.Debug().
			Str(logger.AttrRunID, runID).
			Str(logger.AttrReceiver, rcfg.Type).
			Msg("Receiver started")

		active = append(active, activeReceiver{cfg: rcfg, instance: instance})
	}

	return active, nil
}

func (o *Orchestrator) stopReceivers(active []activeReceiver) {
	for _, r := range active {
		_ = r.instance.Stop()
	}
}

func (o *Orchestrator) collectAndAssertAll(ctx context.Context, runID string, active []activeReceiver, triggerVars map[string]string, result *domain.TestResult, triggerIndex int) {
	var wg sync.WaitGroup
	var mu sync.Mutex

	for _, r := range active {
		wg.Add(1)

		go func(rcfg domain.ReceiverConfig, instance ports.Receiver) {
			defer wg.Done()

			rcvStart := time.Now()
			rcvResult := o.collectAndAssert(ctx, runID, rcfg, instance, triggerVars)
			rcvResult.DurationMs = time.Since(rcvStart).Milliseconds()
			rcvResult.TriggerIndex = triggerIndex

			rcvLevel := slog.LevelDebug
			if rcvResult.Status == domain.StatusFailed || rcvResult.Status == domain.StatusError {
				rcvLevel = slog.LevelWarn
			}

			logEvent := o.logger.Event(rcvLevel).
				Str(logger.AttrRunID, runID).
				Str(logger.AttrTestID, result.TestID).
				Int(logger.AttrStep, triggerIndex+1).
				Str(logger.AttrReceiver, rcvResult.Type).
				Str(logger.AttrStatus, string(rcvResult.Status)).
				Int(logger.AttrDurationMS, int(rcvResult.DurationMs))

			if rcvResult.Error != "" {
				logEvent.Str(logger.AttrError, rcvResult.Error)
			}

			logEvent.Msg("Receiver finished")

			mu.Lock()

			result.Receivers = append(result.Receivers, rcvResult)

			if rcvResult.Status == domain.StatusError && result.Status != domain.StatusError {
				result.Status = domain.StatusError
			} else if rcvResult.Status == domain.StatusFailed && result.Status == domain.StatusPassed {
				result.Status = domain.StatusFailed
			}

			mu.Unlock()
		}(r.cfg, r.instance)
	}

	wg.Wait()

	if result.Error == "" && result.Status != domain.StatusPassed {
		result.Error = summarizeReceiverErrors(result.Receivers)
	}
}

func summarizeReceiverErrors(receivers []domain.ReceiverResult) string {
	var parts []string
	for _, r := range receivers {
		if r.Status == domain.StatusFailed || r.Status == domain.StatusError {
			parts = append(parts, fmt.Sprintf("%s (trigger %d): %s", r.Type, r.TriggerIndex+1, r.Error))
		}
	}

	return strings.Join(parts, "; ")
}

func (o *Orchestrator) collectAndAssert(
	ctx context.Context,
	runID string,
	rcfg domain.ReceiverConfig,
	instance ports.Receiver,
	triggerVars map[string]string,
) domain.ReceiverResult {
	res := domain.ReceiverResult{
		Type:   rcfg.Type,
		Status: domain.StatusPassed,
	}

	collectCtx := ctx
	if rcfg.Timeout > 0 {
		var cancel context.CancelFunc
		collectCtx, cancel = context.WithTimeout(ctx, rcfg.Timeout)
		defer cancel()
	}

	msg, err := instance.Collect(collectCtx)
	if err != nil {
		res.Status = domain.StatusError
		res.Error = fmt.Sprintf("collection failed: %v", err)

		return res
	}

	_ = o.store.Delete(ctx, runID, rcfg.Type)

	res.Message = msg

	for _, acfg := range rcfg.Assertions {
		acfg.Field = template.ReplaceString(acfg.Field, triggerVars)
		acfg.Value = template.ReplaceString(acfg.Value, triggerVars)

		assertion, err := o.assertions.Create(acfg)
		if err != nil {
			res.Status = domain.StatusError
			res.Error = fmt.Sprintf("invalid assertion %s: %v", acfg.Type, err)

			return res
		}

		if err := assertion.Assert(msg); err != nil {
			res.Status = domain.StatusFailed
			res.Error = err.Error()

			return res
		}
	}

	return res
}

func (o *Orchestrator) failResult(result *domain.TestResult, errStr string) {
	result.Status = domain.StatusError
	result.Error = errStr
	result.FinishedAt = time.Now()
	result.DurationMs = result.FinishedAt.Sub(result.StartedAt).Milliseconds()
}

func (o *Orchestrator) notifyFailure(ctx context.Context, cfg domain.OnFailureConfig, result *domain.TestResult) {
	if err := o.notifier.Notify(ctx, cfg, result); err != nil {
		o.logger.Error().
			Str(logger.AttrRunID, result.RunID).
			Str(logger.AttrTestID, result.TestID).
			Err(err).
			Msg("Failure notification dispatch failed")
	}
}

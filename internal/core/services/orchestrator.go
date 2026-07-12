package services

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"e2e-framework/internal/adapters/secondary/assertion"
	"e2e-framework/internal/adapters/secondary/receiver"
	"e2e-framework/internal/core/domain"
	"e2e-framework/internal/core/ports"
	"e2e-framework/internal/pkg/template"
)

type activeReceiver struct {
	cfg      domain.ReceiverConfig
	instance ports.Receiver
}

type Orchestrator struct {
	trigger    ports.Trigger
	store      ports.Store
	receivers  *receiver.ReceiverRegistry
	assertions *assertion.AssertionRegistry
	notifier   ports.Notifier
}

func NewOrchestrator(
	trigger ports.Trigger,
	store ports.Store,
	receivers *receiver.ReceiverRegistry,
	assertions *assertion.AssertionRegistry,
	notifier ports.Notifier,
) *Orchestrator {
	return &Orchestrator{
		trigger:    trigger,
		store:      store,
		receivers:  receivers,
		assertions: assertions,
		notifier:   notifier,
	}
}

func (o *Orchestrator) RunTest(ctx context.Context, def domain.TestDefinition) (string, <-chan *domain.TestResult) {
	runID := fmt.Sprintf("%s-%d", def.ID, time.Now().UnixNano())
	resultCh := make(chan *domain.TestResult, 1)

	go func() {
		resultCh <- o.execute(ctx, def, runID)
	}()

	return runID, resultCh
}

func (o *Orchestrator) execute(ctx context.Context, def domain.TestDefinition, runID string) *domain.TestResult {
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

		return result
	}

	o.executeSequential(ctx, def, runID, result)

	result.FinishedAt = time.Now()
	result.DurationMs = result.FinishedAt.Sub(startTime).Milliseconds()

	if result.Status == domain.StatusFailed || result.Status == domain.StatusError {
		o.notifyFailure(ctx, def.OnFailure, result)
	}

	return result
}

func (o *Orchestrator) executeSequential(ctx context.Context, def domain.TestDefinition, runID string, result *domain.TestResult) {
	triggerVars := make(map[string]string)
	triggerVars["run_id"] = runID

	for i, triggerStep := range def.Triggers {
		reserved, err := o.reserveRecipients(ctx, triggerStep.Receivers, runID)
		if err != nil {
			o.failResult(result, err.Error())

			return
		}

		defer o.releaseRecipients(ctx, reserved)

		stepMaxAttempts := 1
		if def.Retry.Enabled && def.Retry.Attempts > 1 {
			stepMaxAttempts = def.Retry.Attempts
		}

		stepPassed := false

		for attempt := 1; attempt <= stepMaxAttempts; attempt++ {
			if attempt > 1 {
				log.Printf("[%s] step %d retrying (attempt %d/%d)", runID, i+1, attempt, stepMaxAttempts)
			}

			stepVars, triggerErr := o.trigger.Execute(ctx, triggerStep, runID, triggerVars)
			if triggerErr != nil {
				o.failResult(result, fmt.Sprintf("step %d trigger failed: %v", i+1, triggerErr))

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

			active, startErr := o.startReceivers(ctx, triggerStep.Receivers, runID)
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

		if !stepPassed && result.Status != domain.StatusPassed {
			return
		}

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

func (o *Orchestrator) startReceivers(ctx context.Context, configs []domain.ReceiverConfig, runID string) ([]activeReceiver, error) {
	active := make([]activeReceiver, 0, len(configs))

	for _, rcfg := range configs {
		instance, err := o.receivers.Create(rcfg.Type, rcfg.Options)
		if err != nil {
			return active, fmt.Errorf("failed to create receiver %s: %w", rcfg.Type, err)
		}

		if err := instance.Start(ctx, runID); err != nil {
			return active, fmt.Errorf("failed to start receiver %s: %w", rcfg.Type, err)
		}

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
	_ = o.notifier.Notify(ctx, cfg, result)
}

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

	reservedRecipients := make([]domain.ReceiverConfig, 0, len(def.Receivers))

	for _, rcfg := range def.Receivers {
		if rcfg.Recipient != "" {
			if err := o.store.Reserve(ctx, rcfg.Type, rcfg.Recipient, runID); err != nil {
				o.failResult(result, fmt.Sprintf("recipient reservation failed for %s: %v", rcfg.Type, err))

				return result
			}

			reservedRecipients = append(reservedRecipients, rcfg)
		}
	}

	defer func() {
		for _, rcfg := range reservedRecipients {
			_ = o.store.Release(ctx, rcfg.Type, rcfg.Recipient)
		}
	}()

	maxAttempts := 1
	if def.Retry.Enabled && def.Retry.Attempts > 1 {
		maxAttempts = def.Retry.Attempts
	}

	for attempt := 1; attempt <= maxAttempts; attempt++ {
		if attempt > 1 {
			log.Printf("[%s] retrying (attempt %d/%d) after previous failure", runID, attempt, maxAttempts)
		}

		result.Attempts = attempt
		result.Receivers = make([]domain.ReceiverResult, 0, len(def.Receivers))
		result.Status = domain.StatusPassed
		result.Error = ""
		result.TriggerVars = nil

		activeReceivers := make([]struct {
			cfg      domain.ReceiverConfig
			instance ports.Receiver
		}, 0, len(def.Receivers))

		configErr := false

		for _, rcfg := range def.Receivers {
			instance, err := o.receivers.Create(rcfg.Type, rcfg.Options)
			if err != nil {
				o.failResult(result, fmt.Sprintf("failed to create receiver %s: %v", rcfg.Type, err))
				configErr = true

				break
			}

			if err := instance.Start(ctx, runID); err != nil {
				o.failResult(result, fmt.Sprintf("failed to start receiver %s: %v", rcfg.Type, err))
				configErr = true

				break
			}

			activeReceivers = append(activeReceivers, struct {
				cfg      domain.ReceiverConfig
				instance ports.Receiver
			}{cfg: rcfg, instance: instance})
		}

		stopActiveReceivers := func() {
			for _, r := range activeReceivers {
				_ = r.instance.Stop()
			}
		}

		if configErr {
			stopActiveReceivers()

			break
		}

		triggerVars, err := o.trigger.Execute(ctx, def.Trigger, runID)
		if err != nil {
			o.failResult(result, fmt.Sprintf("trigger failed: %v", err))
			stopActiveReceivers()

			if attempt < maxAttempts {
				time.Sleep(def.Retry.Delay)
			}

			continue
		}

		result.TriggerVars = triggerVars

		var wg sync.WaitGroup
		var mu sync.Mutex

		for _, r := range activeReceivers {
			wg.Add(1)

			go func(rcfg domain.ReceiverConfig, instance ports.Receiver) {
				defer wg.Done()

				rcvStart := time.Now()
				rcvResult := o.collectAndAssert(ctx, runID, rcfg, instance, triggerVars)
				rcvResult.DurationMs = time.Since(rcvStart).Milliseconds()

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
		stopActiveReceivers()

		if result.Status == domain.StatusPassed {
			break
		}

		if attempt < maxAttempts {
			time.Sleep(def.Retry.Delay)
		}
	}

	result.FinishedAt = time.Now()
	result.DurationMs = result.FinishedAt.Sub(startTime).Milliseconds()

	if result.Status == domain.StatusFailed || result.Status == domain.StatusError {
		o.notifyFailure(ctx, def.OnFailure, result)
	}

	return result
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

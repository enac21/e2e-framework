package services

import (
	"context"
	"fmt"
	"sync"
	"time"

	"e2e-framework/internal/adapters/secondary/assertion"
	"e2e-framework/internal/adapters/secondary/receiver"
	"e2e-framework/internal/core/domain"
	"e2e-framework/internal/core/ports"
)

type Orchestrator struct {
	trigger    ports.Trigger
	receivers  *receiver.ReceiverRegistry
	assertions *assertion.AssertionRegistry
	notifier   ports.Notifier
}

func NewOrchestrator(
	trigger ports.Trigger,
	receivers *receiver.ReceiverRegistry,
	assertions *assertion.AssertionRegistry,
	notifier ports.Notifier,
) *Orchestrator {
	return &Orchestrator{
		trigger:    trigger,
		receivers:  receivers,
		assertions: assertions,
		notifier:   notifier,
	}
}

func (o *Orchestrator) RunTest(ctx context.Context, def domain.TestDefinition) *domain.TestResult {
	startTime := time.Now()
	runID := fmt.Sprintf("%s-%d", def.ID, startTime.UnixNano())

	result := &domain.TestResult{
		TestID:     def.ID,
		RunID:      runID,
		Status:     domain.StatusPassed,
		StartedAt:  startTime,
		Receivers:  make([]domain.ReceiverResult, 0, len(def.Receivers)),
	}

	if !def.Enabled {
		result.Status = domain.StatusSkipped
		result.FinishedAt = time.Now()
		result.DurationMs = result.FinishedAt.Sub(startTime).Milliseconds()
		return result
	}

	activeReceivers := make([]struct {
		cfg      domain.ReceiverConfig
		instance ports.Receiver
	}, 0, len(def.Receivers))

	// Ensure all started receivers are stopped
	defer func() {
		for _, r := range activeReceivers {
			_ = r.instance.Stop()
		}
	}()

	// 1. Initialize and Start Receivers
	for _, rcfg := range def.Receivers {
		instance, err := o.receivers.Create(rcfg.Type)
		if err != nil {
			o.failResult(result, fmt.Sprintf("failed to create receiver %s: %v", rcfg.Type, err))
			return result
		}

		if err := instance.Start(ctx, runID); err != nil {
			o.failResult(result, fmt.Sprintf("failed to start receiver %s: %v", rcfg.Type, err))
			return result
		}

		activeReceivers = append(activeReceivers, struct {
			cfg      domain.ReceiverConfig
			instance ports.Receiver
		}{cfg: rcfg, instance: instance})
	}

	// 2. Execute Trigger
	if err := o.trigger.Execute(ctx, def.Trigger, runID); err != nil {
		o.failResult(result, fmt.Sprintf("trigger failed: %v", err))
		o.notifyFailure(ctx, def.OnFailure, result)
		return result
	}

	// 3. Collect and Assert Concurrently
	var wg sync.WaitGroup
	var mu sync.Mutex

	for _, r := range activeReceivers {
		wg.Add(1)
		go func(rcfg domain.ReceiverConfig, instance ports.Receiver) {
			defer wg.Done()
			rcvStart := time.Now()
			rcvResult := o.collectAndAssert(ctx, runID, rcfg, instance)
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

	result.FinishedAt = time.Now()
	result.DurationMs = result.FinishedAt.Sub(startTime).Milliseconds()

	// 4. Notify if failed
	if result.Status == domain.StatusFailed || result.Status == domain.StatusError {
		o.notifyFailure(ctx, def.OnFailure, result)
	}

	return result
}

func (o *Orchestrator) collectAndAssert(ctx context.Context, runID string, rcfg domain.ReceiverConfig, instance ports.Receiver) domain.ReceiverResult {
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

	res.Message = msg

	for _, acfg := range rcfg.Assertions {
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
	// Notifier is fire-and-forget; we don't return its error to the caller,
	// but in a real system we would log it.
	_ = o.notifier.Notify(ctx, cfg, result)
}

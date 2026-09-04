package trigger

import (
	"context"
	"errors"
	"testing"

	"e2e-framework/internal/core/domain"
	"e2e-framework/internal/core/ports"
)

func TestTriggerRegistry_RegisterAndCreate(t *testing.T) {
	reg := NewTriggerRegistry()

	var gotOptions map[string]string
	reg.Register("fake", func(options map[string]string) (ports.Trigger, error) {
		gotOptions = options
		return fakeTrigger{}, nil
	})

	triggerInstance, err := reg.Create("fake", map[string]string{"timeout": "5s"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if triggerInstance == nil {
		t.Fatal("expected a trigger instance")
	}

	if gotOptions["timeout"] != "5s" {
		t.Errorf("expected options passed to factory, got %v", gotOptions)
	}
}

func TestTriggerRegistry_CreateUnknownType(t *testing.T) {
	reg := NewTriggerRegistry()

	_, err := reg.Create("smtp", nil)
	if err == nil {
		t.Fatal("expected error")
	}

	if !errors.Is(err, domain.ErrConfiguration) {
		t.Fatalf("want ErrConfiguration, got %v", err)
	}
}

func TestTriggerRegistry_FactoryErrorPropagates(t *testing.T) {
	reg := NewTriggerRegistry()
	reg.Register("broken", func(options map[string]string) (ports.Trigger, error) {
		return nil, errors.New("factory boom")
	})

	_, err := reg.Create("broken", nil)
	if err == nil || err.Error() != "factory boom" {
		t.Fatalf("expected factory error to propagate, got %v", err)
	}
}

type fakeTrigger struct{}

func (fakeTrigger) Execute(_ context.Context, _ domain.TriggerConfig, _ string, _ map[string]string) (map[string]string, error) {
	return map[string]string{}, nil
}

package store

import (
	"context"
	"errors"
	"testing"

	"e2e-framework/internal/core/domain"
	"e2e-framework/internal/core/ports"
)

type fakeStore struct{}

func (fakeStore) Deposit(context.Context, *domain.Message) error { return nil }
func (fakeStore) Claim(context.Context, string, string) (*domain.Message, error) {
	return nil, nil
}
func (fakeStore) Reserve(context.Context, string, string, string) error { return nil }
func (fakeStore) Release(context.Context, string, string) error         { return nil }
func (fakeStore) Delete(context.Context, string, string) error          { return nil }
func (fakeStore) Close() error                                          { return nil }

func TestStoreRegistry_RegisterAndCreate(t *testing.T) {
	reg := NewStoreRegistry()

	var gotCfg StoreConfig
	reg.Register("fake", func(cfg StoreConfig) (ports.Store, error) {
		gotCfg = cfg
		return fakeStore{}, nil
	})

	instance, err := reg.Create("fake", StoreConfig{Type: "fake"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if instance == nil {
		t.Fatal("expected a store instance")
	}

	if gotCfg.Type != "fake" {
		t.Errorf("expected config passed to factory, got %+v", gotCfg)
	}
}

func TestStoreRegistry_CreateUnknownType(t *testing.T) {
	reg := NewStoreRegistry()

	_, err := reg.Create("mongo", StoreConfig{Type: "mongo"})
	if err == nil {
		t.Fatal("expected error")
	}

	if !errors.Is(err, domain.ErrConfiguration) {
		t.Fatalf("want ErrConfiguration, got %v", err)
	}
}

func TestStoreRegistry_FactoryErrorPropagates(t *testing.T) {
	reg := NewStoreRegistry()
	reg.Register("broken", func(cfg StoreConfig) (ports.Store, error) {
		return nil, errors.New("factory boom")
	})

	_, err := reg.Create("broken", StoreConfig{})
	if err == nil || err.Error() != "factory boom" {
		t.Fatalf("expected factory error to propagate, got %v", err)
	}
}

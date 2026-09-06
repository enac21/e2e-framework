package store

import (
	"errors"
	"testing"

	"go.uber.org/mock/gomock"

	"e2e-framework/internal/core/domain"
	"e2e-framework/internal/core/ports"
	"e2e-framework/internal/core/ports/mocks"
	"e2e-framework/internal/pkg/config"
)

func TestStoreRegistry_RegisterAndCreate(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	reg := NewStoreRegistry()

	var gotCfg config.StoreConfig
	reg.Register("fake", func(cfg config.StoreConfig) (ports.Store, error) {
		gotCfg = cfg
		return mocks.NewMockStore(ctrl), nil
	})

	instance, err := reg.Create("fake", config.StoreConfig{Type: "fake"})
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

	_, err := reg.Create("mongo", config.StoreConfig{Type: "mongo"})
	if err == nil {
		t.Fatal("expected error")
	}

	if !errors.Is(err, domain.ErrConfiguration) {
		t.Fatalf("want ErrConfiguration, got %v", err)
	}
}

func TestStoreRegistry_FactoryErrorPropagates(t *testing.T) {
	reg := NewStoreRegistry()
	reg.Register("broken", func(cfg config.StoreConfig) (ports.Store, error) {
		return nil, errors.New("factory boom")
	})

	_, err := reg.Create("broken", config.StoreConfig{})
	if err == nil || err.Error() != "factory boom" {
		t.Fatalf("expected factory error to propagate, got %v", err)
	}
}

package domain

import (
	"testing"

	"gopkg.in/yaml.v3"
)

func TestTriggerConfig_EffectiveType(t *testing.T) {
	if got := (TriggerConfig{}).EffectiveType(); got != HTTPTriggerType {
		t.Errorf("default EffectiveType = %q, want %q", got, HTTPTriggerType)
	}

	if got := (TriggerConfig{Type: "smtp"}).EffectiveType(); got != "smtp" {
		t.Errorf("EffectiveType = %q, want smtp", got)
	}
}

func TestTriggerConfig_YAMLDecodesTypeAndOptions(t *testing.T) {
	var cfg TriggerConfig
	raw := []byte("type: smtp\nurl: mail://host\noptions:\n  host: h1\n  port: 587\n")
	if err := yaml.Unmarshal(raw, &cfg); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if cfg.Type != "smtp" {
		t.Errorf("Type = %q, want smtp", cfg.Type)
	}

	if cfg.Options["host"] != "h1" || cfg.Options["port"] != "587" {
		t.Errorf("Options not decoded: %v", cfg.Options)
	}
}

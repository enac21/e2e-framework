package domain

import "time"

type TestDefinition struct {
	Version     string           `yaml:"version"`
	ID          string           `yaml:"id"`
	Description string           `yaml:"description"`
	Schedule    string           `yaml:"schedule"`
	Enabled     bool             `yaml:"enabled"`
	Retry       RetryConfig      `yaml:"retry"`
	Trigger     TriggerConfig    `yaml:"trigger"`
	Receivers   []ReceiverConfig `yaml:"receivers"`
	OnFailure   OnFailureConfig  `yaml:"on_failure"`
}

type RetryConfig struct {
	Enabled  bool          `yaml:"enabled"`
	Attempts int           `yaml:"attempts"`
	Delay    time.Duration `yaml:"delay"`
}

type TriggerConfig struct {
	Method  string            `yaml:"method"`
	URL     string            `yaml:"url"`
	Timeout time.Duration     `yaml:"timeout"`
	Headers map[string]string `yaml:"headers"`
	Body    map[string]any    `yaml:"body"`
}

type ReceiverConfig struct {
	Type       string            `yaml:"type"`
	Timeout    time.Duration     `yaml:"timeout"`
	Assertions []AssertionConfig `yaml:"assertions"`
}

type AssertionConfig struct {
	Type  string `yaml:"type"`
	Field string `yaml:"field"`
	Value string `yaml:"value"`
}

type OnFailureConfig struct {
	Webhook WebhookAction `yaml:"webhook"`
}

type WebhookAction struct {
	URL     string            `yaml:"url"`
	Method  string            `yaml:"method"`
	Headers map[string]string `yaml:"headers"`
	Body    map[string]any    `yaml:"body"`
}

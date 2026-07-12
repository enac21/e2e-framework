package domain

import (
	"fmt"
	"time"

	"gopkg.in/yaml.v3"
)

type TestDefinition struct {
	Version   string           `yaml:"version"`
	ID        string           `yaml:"id"`
	Schedule  string           `yaml:"schedule"`
	Enabled   bool             `yaml:"enabled"`
	Async     bool             `yaml:"async"`
	Retry     RetryConfig      `yaml:"retry"`
	Triggers  []TriggerConfig  `yaml:"triggers"`
	OnFailure OnFailureConfig  `yaml:"on_failure"`
}

type RetryConfig struct {
	Enabled  bool          `yaml:"enabled"`
	Attempts int           `yaml:"attempts"`
	Delay    time.Duration `yaml:"delay"`
}

type TriggerConfig struct {
	Method           string            `yaml:"method"`
	URL              string            `yaml:"url"`
	Timeout          time.Duration     `yaml:"timeout"`
	Headers          map[string]string `yaml:"headers"`
	Body             map[string]any    `yaml:"body"`
	Extract          map[string]string `yaml:"extract"`
	Receivers        []ReceiverConfig  `yaml:"receivers"`
	WaitForReceivers bool              `yaml:"wait_for_receivers"`
}

type ReceiverConfig struct {
	Type       string            `yaml:"type"`
	Timeout    time.Duration     `yaml:"timeout"`
	Recipient  string            `yaml:"recipient"`
	Options    OptionsMap        `yaml:"options"`
	Assertions []AssertionConfig `yaml:"assertions"`
}

type OptionsMap map[string]string

func (o *OptionsMap) UnmarshalYAML(value *yaml.Node) error {
	raw := make(map[string]any)
	if err := value.Decode(&raw); err != nil {
		return err
	}

	result := make(OptionsMap, len(raw))
	for k, v := range raw {
		result[k] = fmt.Sprintf("%v", v)
	}

	*o = result

	return nil
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

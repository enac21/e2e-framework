package domain

import (
	"fmt"
	"time"

	"gopkg.in/yaml.v3"
)

type TestDefinition struct {
	Version   string            `yaml:"version"`
	ID        string            `yaml:"id"`
	Schedule  string            `yaml:"schedule"`
	Enabled   bool              `yaml:"enabled"`
	Async     bool              `yaml:"async"`
	Retry     RetryConfig       `yaml:"retry"`
	Variables map[string]string `yaml:"variables"`
	Triggers  []TriggerConfig   `yaml:"triggers"`
	OnFailure OnFailureConfig   `yaml:"on_failure"`
}

type RetryConfig struct {
	Enabled  bool          `yaml:"enabled"`
	Attempts int           `yaml:"attempts"`
	Delay    time.Duration `yaml:"delay"`
}

type TriggerConfig struct {
	Type               string            `yaml:"type"`
	Options            OptionsMap        `yaml:"options"`
	Method             string            `yaml:"method"`
	URL                string            `yaml:"url"`
	Timeout            time.Duration     `yaml:"timeout"`
	DelayBefore        time.Duration     `yaml:"delay_before"`
	Headers            map[string]string `yaml:"headers"`
	Body               map[string]any    `yaml:"body"`
	Extract            map[string]string `yaml:"extract"`
	ExpectedStatus     int               `yaml:"expected_status"`
	ResponseAssertions []AssertionConfig `yaml:"response_assertions"`
	Receivers          []ReceiverConfig  `yaml:"receivers"`
	WaitForReceivers   bool              `yaml:"wait_for_receivers"`
}

// EffectiveType returns the trigger type to use, defaulting to HTTP when
// the step does not declare one.
func (t TriggerConfig) EffectiveType() string {
	if t.Type == "" {
		return HTTPTriggerType
	}

	return t.Type
}

type ReceiverConfig struct {
	Type       string            `yaml:"type"`
	Timeout    time.Duration     `yaml:"timeout"`
	Recipient  string            `yaml:"recipient"`
	Options    OptionsMap        `yaml:"options"`
	Assertions []AssertionConfig `yaml:"assertions"`

	// Trigger-like fields used by the "api" receiver (outbound HTTP polling).
	Interval        time.Duration     `yaml:"interval"`
	Method          string            `yaml:"method"`
	URL             string            `yaml:"url"`
	Headers         map[string]string `yaml:"headers"`
	Body            map[string]any    `yaml:"body"`
	ExpectedStatus  int               `yaml:"expected_status"`
	ResponseAsserts []AssertionConfig `yaml:"response_assertions"`
	Extract         map[string]string `yaml:"extract"`
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
	Calls []CallAction `yaml:"calls"`
}

// CallAction is a single outbound HTTP call executed when a test fails.
// Its shape mirrors the core fields of a TriggerConfig.
type CallAction struct {
	Method         string            `yaml:"method"`
	URL            string            `yaml:"url"`
	Timeout        time.Duration     `yaml:"timeout"`
	DelayBefore    time.Duration     `yaml:"delay_before"`
	Headers        map[string]string `yaml:"headers"`
	Body           map[string]any    `yaml:"body"`
	ExpectedStatus int               `yaml:"expected_status"`
}

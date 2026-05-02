package config

import (
	"fmt"
	"os"
	"regexp"

	"gopkg.in/yaml.v3"

	"e2e-framework/internal/core/domain"
)

type Config struct {
	Server struct {
		Port int `yaml:"port"`
	} `yaml:"server"`
	Store struct {
		Redis struct {
			URL string `yaml:"url"`
		} `yaml:"redis"`
	} `yaml:"store"`
	Webhook struct {
		Port int `yaml:"port"`
	} `yaml:"webhook"`
}

var envRegex = regexp.MustCompile(`\{\{env\.([^}]+)\}\}`)

func LoadConfig(path string) (*Config, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("%w: failed to read config file: %v", domain.ErrConfiguration, err)
	}

	// Resolve env variables before parsing
	resolved := envRegex.ReplaceAllStringFunc(string(b), func(match string) string {
		submatch := envRegex.FindStringSubmatch(match)
		if len(submatch) == 2 {
			return os.Getenv(submatch[1])
		}
		return match
	})

	var cfg Config
	if err := yaml.Unmarshal([]byte(resolved), &cfg); err != nil {
		return nil, fmt.Errorf("%w: failed to parse config yaml: %v", domain.ErrConfiguration, err)
	}

	return &cfg, nil
}

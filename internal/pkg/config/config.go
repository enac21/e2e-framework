package config

import (
	"fmt"
	"os"
	"regexp"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Store struct {
		RedisURL string `yaml:"redis_url"`
	} `yaml:"store"`
	API struct {
		Port int `yaml:"port"`
	} `yaml:"api"`
	Webhook struct {
		Port int `yaml:"port"`
	} `yaml:"webhook"`
	Receivers map[string]map[string]any `yaml:"receivers"`
}

var envRegex = regexp.MustCompile(`\{\{env\.([^}]+)\}\}`)

func LoadConfig(path string) (*Config, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
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
		return nil, fmt.Errorf("failed to parse config yaml: %w", err)
	}

	return &cfg, nil
}

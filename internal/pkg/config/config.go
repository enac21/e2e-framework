package config

import (
	"fmt"
	"os"
	"regexp"
	"time"

	"gopkg.in/yaml.v3"

	"e2e-framework/internal/core/domain"
)

const StoreTypeDefault = "redis"

type RedisStoreConfig struct {
	URL         string        `yaml:"url"`
	TTL         time.Duration `yaml:"ttl"`
	Username    string        `yaml:"username"`
	Password    string        `yaml:"password"`
	ClusterMode bool          `yaml:"cluster_mode"`
}

type PostgresStoreConfig struct {
	DSN string        `yaml:"dsn"`
	TTL time.Duration `yaml:"ttl"`
}

type MemoryStoreConfig struct {
	TTL time.Duration `yaml:"ttl"`
}

type TestGroupConfig struct {
	Description  string        `yaml:"description"`
	Tests        []string      `yaml:"tests"`
	TestDelay    time.Duration `yaml:"test_delay"`
	SkipFailTest bool          `yaml:"skip_fail_test"`
}

type StoreConfig struct {
	Type     string              `yaml:"type"`
	Redis    RedisStoreConfig    `yaml:"redis"`
	Postgres PostgresStoreConfig `yaml:"postgres"`
	Memory   MemoryStoreConfig   `yaml:"memory"`
}

type LoggingConfig struct {
	Level  string `yaml:"level"`
	Format string `yaml:"format"`
}

type Config struct {
	Server struct {
		Port int `yaml:"port"`
	} `yaml:"server"`
	Auth struct {
		Enabled   bool   `yaml:"enabled"`
		JWTSecret string `yaml:"jwt_secret"`
	} `yaml:"auth"`
	Store      StoreConfig                `yaml:"store"`
	Logging    LoggingConfig              `yaml:"logging"`
	TestGroups map[string]TestGroupConfig `yaml:"test_groups"`
	Tests      struct {
		Path string `yaml:"path"`
	} `yaml:"tests"`
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

	if cfg.Store.Type == "" {
		cfg.Store.Type = StoreTypeDefault
	}

	if envType := os.Getenv("STORE_TYPE"); envType != "" {
		cfg.Store.Type = envType
	}

	if cfg.Store.Redis.TTL == 0 {
		cfg.Store.Redis.TTL = 300 * time.Second
	}

	if cfg.Store.Postgres.TTL == 0 {
		cfg.Store.Postgres.TTL = 300 * time.Second
	}

	if cfg.Store.Memory.TTL == 0 {
		cfg.Store.Memory.TTL = 300 * time.Second
	}

	if cfg.Logging.Level == "" {
		cfg.Logging.Level = "info"
	}

	if cfg.Logging.Format == "" {
		cfg.Logging.Format = "json"
	}

	if err := validateLogging(cfg.Logging); err != nil {
		return nil, err
	}

	return &cfg, nil
}

func validateLogging(l LoggingConfig) error {
	switch l.Level {
	case "debug", "info", "warn", "error":
	default:
		return fmt.Errorf("%w: unknown logging.level %q (supported: debug, info, warn, error)", domain.ErrConfiguration, l.Level)
	}

	switch l.Format {
	case "json", "text":
	default:
		return fmt.Errorf("%w: unknown logging.format %q (supported: json, text)", domain.ErrConfiguration, l.Format)
	}

	return nil
}

func ValidateTestGroups(groups map[string]TestGroupConfig, tests map[string]domain.TestDefinition) error {
	for name, group := range groups {
		if len(group.Tests) == 0 {
			return fmt.Errorf("%w: test group %q has no tests", domain.ErrConfiguration, name)
		}

		for _, id := range group.Tests {
			if _, ok := tests[id]; !ok {
				return fmt.Errorf("%w: test group %q references unknown test %q", domain.ErrConfiguration, name, id)
			}
		}
	}

	return nil
}

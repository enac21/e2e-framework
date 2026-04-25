package config

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"

	"e2e-framework/internal/core/domain"
)

func LoadTestDefinitions(dir string) (map[string]domain.TestDefinition, error) {
	tests := make(map[string]domain.TestDefinition)

	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("failed to read test directory: %w", err)
	}

	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".yaml" {
			continue
		}

		path := filepath.Join(dir, entry.Name())
		b, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("failed to read test file %s: %w", path, err)
		}

		// Resolve env vars
		resolved := envRegex.ReplaceAllStringFunc(string(b), func(match string) string {
			submatch := envRegex.FindStringSubmatch(match)
			if len(submatch) == 2 {
				return os.Getenv(submatch[1])
			}
			return match
		})

		var def domain.TestDefinition
		if err := yaml.Unmarshal([]byte(resolved), &def); err != nil {
			return nil, fmt.Errorf("failed to parse test file %s: %w", path, err)
		}

		tests[def.ID] = def
	}

	return tests, nil
}

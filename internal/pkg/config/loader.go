package config

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"

	"e2e-framework/internal/core/domain"
)

func LoadTestDefinitions(dir string) (map[string]domain.TestDefinition, error) {
	tests := make(map[string]domain.TestDefinition)

	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || filepath.Ext(d.Name()) != ".yaml" {
			return nil
		}

		b, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("%w: failed to read test file %s: %v", domain.ErrConfiguration, path, err)
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
			return fmt.Errorf("%w: failed to parse test file %s: %v", domain.ErrConfiguration, path, err)
		}

		tests[def.ID] = def
		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("%w: failed to walk test directory: %v", domain.ErrConfiguration, err)
	}

	return tests, nil
}

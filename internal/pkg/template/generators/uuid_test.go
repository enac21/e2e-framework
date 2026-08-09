package generators

import (
	"regexp"
	"testing"
)

var uuidV4Regex = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)

func TestUUIDGenerator_Generate_ValidFormat(t *testing.T) {
	gen := UUIDGenerator{}
	for range 20 {
		result, ok := gen.Generate("")
		if !ok {
			t.Fatal("expected ok=true for empty args")
		}

		if !uuidV4Regex.MatchString(result) {
			t.Errorf("expected a valid v4 UUID, got: %q", result)
		}
	}
}

func TestUUIDGenerator_Generate_RejectsArgs(t *testing.T) {
	gen := UUIDGenerator{}
	for _, args := range []string{"v4", " ", "anything"} {
		if result, ok := gen.Generate(args); ok {
			t.Errorf("args=%q: expected ok=false, got %q", args, result)
		}
	}
}

func TestUUIDGenerator_Generate_IndependentValues(t *testing.T) {
	gen := UUIDGenerator{}
	seen := make(map[string]bool)
	for range 50 {
		result, ok := gen.Generate("")
		if !ok {
			t.Fatal("expected ok=true")
		}

		if seen[result] {
			t.Fatalf("duplicate UUID generated: %q", result)
		}

		seen[result] = true
	}
}

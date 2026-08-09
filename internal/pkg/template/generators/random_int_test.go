package generators

import (
	"strconv"
	"testing"
)

func TestRandomIntGenerator_Generate_InRange(t *testing.T) {
	cases := []struct {
		digits int
		max    int
	}{
		{1, 10},
		{4, 10000},
		{6, 1000000},
	}

	gen := RandomIntGenerator{}
	for _, tc := range cases {
		for range 20 {
			result, ok := gen.Generate(strconv.Itoa(tc.digits))
			if !ok {
				t.Errorf("digits=%d: expected ok=true", tc.digits)
				continue
			}

			n, err := strconv.Atoi(result)
			if err != nil {
				t.Errorf("digits=%d: not a valid int: %q", tc.digits, result)
				continue
			}

			if n < 0 || n >= tc.max {
				t.Errorf("digits=%d: value %d out of range [0, %d)", tc.digits, n, tc.max)
			}
		}
	}
}

func TestRandomIntGenerator_Generate_InvalidArgs(t *testing.T) {
	gen := RandomIntGenerator{}
	for _, args := range []string{"", "abc", "0", "-3", "  "} {
		if result, ok := gen.Generate(args); ok {
			t.Errorf("args=%q: expected ok=false, got %q", args, result)
		}
	}
}

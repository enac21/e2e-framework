package template

import (
	"strconv"
	"strings"
	"testing"
)

func TestReplaceString_KnownVars(t *testing.T) {
	vars := map[string]string{"run_id": "abc123", "name": "test"}
	got := ReplaceString("run={{run_id}} name={{name}}", vars)
	if got != "run=abc123 name=test" {
		t.Errorf("unexpected: %s", got)
	}
}

func TestReplaceString_UnknownTagUntouched(t *testing.T) {
	got := ReplaceString("{{unknown}}", map[string]string{})
	if got != "{{unknown}}" {
		t.Errorf("unknown tag should be untouched, got: %s", got)
	}
}

func TestReplaceString_RandomInt_InRange(t *testing.T) {
	cases := []struct {
		digits int
		max    int
	}{
		{1, 10},
		{4, 10000},
		{6, 1000000},
	}

	for _, tc := range cases {
		tag := "{{randomInt(" + strconv.Itoa(tc.digits) + ")}}"
		for range 20 {
			got := ReplaceString(tag, map[string]string{})
			n, err := strconv.Atoi(got)
			if err != nil {
				t.Errorf("digits=%d: not a valid int: %q", tc.digits, got)
				continue
			}

			if n < 0 || n >= tc.max {
				t.Errorf("digits=%d: value %d out of range [0, %d)", tc.digits, n, tc.max)
			}
		}
	}
}

func TestReplaceString_RandomInt_IndependentOccurrences(t *testing.T) {
	s := "{{randomInt(9)}} {{randomInt(9)}}"
	differentFound := false

	for range 50 {
		got := ReplaceString(s, map[string]string{})
		parts := strings.Fields(got)
		if len(parts) != 2 {
			t.Fatalf("unexpected format: %q", got)
		}

		if parts[0] != parts[1] {
			differentFound = true
			break
		}
	}

	if !differentFound {
		t.Error("expected independent randomInt occurrences to differ at least once in 50 runs")
	}
}

func TestReplaceString_Mixed_VarsAndRandomInt(t *testing.T) {
	vars := map[string]string{"run_id": "xyz"}
	got := ReplaceString("id={{run_id}} code={{randomInt(3)}}", vars)
	if !strings.HasPrefix(got, "id=xyz code=") {
		t.Errorf("unexpected: %s", got)
	}

	suffix := strings.TrimPrefix(got, "id=xyz code=")
	n, err := strconv.Atoi(suffix)
	if err != nil {
		t.Errorf("code part not a valid int: %q", suffix)
	}

	if n < 0 || n >= 1000 {
		t.Errorf("code %d out of range [0, 1000)", n)
	}
}

func TestReplaceMap_RandomInt_InBody(t *testing.T) {
	body := map[string]any{
		"idempotency_key": "{{randomInt(8)}}",
		"name":            "{{name}}",
	}
	vars := map[string]string{"name": "alice"}
	result := ReplaceMap(body, vars)

	name, _ := result["name"].(string)
	if name != "alice" {
		t.Errorf("name: expected alice, got %s", name)
	}

	keyStr, _ := result["idempotency_key"].(string)
	n, err := strconv.Atoi(keyStr)
	if err != nil {
		t.Errorf("idempotency_key not a valid int: %q", keyStr)
	}

	if n < 0 || n >= 100000000 {
		t.Errorf("idempotency_key %d out of range", n)
	}
}

func TestHasUnresolved_ResolvedString(t *testing.T) {
	if HasUnresolved("run=abc123 name=alice") {
		t.Error("fully resolved string should not be reported as unresolved")
	}
}

func TestHasUnresolved_RemainingPlaceholder(t *testing.T) {
	if !HasUnresolved("run={{run_id}} name={{missing}}") {
		t.Error("string with a remaining placeholder should be reported as unresolved")
	}
}

func TestHasUnresolved_EmptyString(t *testing.T) {
	if HasUnresolved("") {
		t.Error("empty string should not be reported as unresolved")
	}
}

func benchmarkPayload(small bool) string {
	if small {
		return "test_id={{test_id}} run_id={{run_id}} error={{error}} transaction_id={{transaction_id}}"
	}

	parts := make([]string, 0, 100)
	for i := range 100 {
		parts = append(parts, "field_"+strconv.Itoa(i)+"={{run_id}}_{{transaction_id}}")
	}

	return strings.Join(parts, " ")
}

func BenchmarkReplaceString(b *testing.B) {
	vars := map[string]string{
		"run_id":         "abc",
		"test_id":        "t1",
		"error":          "boom",
		"transaction_id": "txn-1",
	}

	cases := []struct {
		name      string
		payload   string
		withRegex bool
	}{
		{name: "small/without-has-unresolved", payload: benchmarkPayload(true), withRegex: false},
		{name: "small/with-has-unresolved", payload: benchmarkPayload(true), withRegex: true},
		{name: "large/without-has-unresolved", payload: benchmarkPayload(false), withRegex: false},
		{name: "large/with-has-unresolved", payload: benchmarkPayload(false), withRegex: true},
	}

	for _, tc := range cases {
		b.Run(tc.name, func(b *testing.B) {
			for b.Loop() {
				resolved := ReplaceString(tc.payload, vars)
				if tc.withRegex {
					HasUnresolved(resolved)
				}
			}
		})
	}
}

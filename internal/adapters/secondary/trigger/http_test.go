package trigger

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/tidwall/gjson"

	"e2e-framework/internal/core/domain"
	"e2e-framework/internal/pkg/httputil"
)

func mustJSON(v any) []byte {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return b
}

func TestWalkFind(t *testing.T) {
	tests := []struct {
		name   string
		json   string
		path   string
		target string
		want   bool
	}{
		{
			name:   "flat array hit",
			json:   `{"items":["a","b","c"]}`,
			path:   "items",
			target: "b",
			want:   true,
		},
		{
			name:   "flat array miss",
			json:   `{"items":["a","b","c"]}`,
			path:   "items",
			target: "d",
			want:   false,
		},
		{
			name:   "array of objects nested field",
			json:   `{"items":[{"name":"Alice"},{"name":"Bob"}]}`,
			path:   "items.#.name",
			target: "Alice",
			want:   true,
		},
		{
			name:   "doubly nested arrays",
			json:   `{"data":[{"statuses":[{"general_status":"requested"},{"general_status":"sending"}]}]}`,
			path:   "data.#.statuses.#.general_status",
			target: "requested",
			want:   true,
		},
		{
			name:   "doubly nested miss",
			json:   `{"data":[{"statuses":[{"general_status":"requested"},{"general_status":"sending"}]}]}`,
			path:   "data.#.statuses.#.general_status",
			target: "delivered",
			want:   false,
		},
		{
			name:   "map values wildcard hit",
			json:   `{"labels":{"env":"prod","tier":"web"}}`,
			path:   "labels.@values",
			target: "prod",
			want:   true,
		},
		{
			name:   "map values wildcard miss",
			json:   `{"labels":{"env":"prod","tier":"web"}}`,
			path:   "labels.@values",
			target: "staging",
			want:   false,
		},
		{
			name:   "scalar result",
			json:   `{"status":"active"}`,
			path:   "status",
			target: "active",
			want:   true,
		},
		{
			name:   "missing path",
			json:   `{"status":"active"}`,
			path:   "nonexistent",
			target: "active",
			want:   false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := walkFind(gjson.Get(tc.json, tc.path), tc.target)
			if got != tc.want {
				t.Errorf("walkFind(%q, %q) = %v, want %v", tc.path, tc.target, got, tc.want)
			}
		})
	}
}

func TestRunResponseAssertions(t *testing.T) {
	type assertion struct {
		typ   string
		field string
		value string
	}

	run := func(t *testing.T, payload any, vars map[string]string, a assertion) error {
		t.Helper()
		raw := mustJSON(payload)
		var m map[string]any
		_ = json.Unmarshal(raw, &m)
		flat := httputil.FlattenJSON(m)
		return runResponseAssertions(
			[]domain.AssertionConfig{{Type: a.typ, Field: a.field, Value: a.value}},
			flat, raw, vars,
		)
	}

	t.Run("equals pass", func(t *testing.T) {
		if err := run(t, map[string]any{"status": "ok"}, nil, assertion{"equals", "status", "ok"}); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("equals fail", func(t *testing.T) {
		err := run(t, map[string]any{"status": "ok"}, nil, assertion{"equals", "status", "fail"})
		if err == nil {
			t.Fatal("expected error")
		}
		if !errors.Is(err, domain.ErrTriggerFailed) {
			t.Fatalf("want ErrTriggerFailed, got %v", err)
		}
	})

	t.Run("contains pass", func(t *testing.T) {
		if err := run(t, map[string]any{"msg": "hello world"}, nil, assertion{"contains", "msg", "world"}); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("contains fail", func(t *testing.T) {
		if err := run(t, map[string]any{"msg": "hello"}, nil, assertion{"contains", "msg", "world"}); err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("not_contains pass", func(t *testing.T) {
		if err := run(t, map[string]any{"msg": "hello"}, nil, assertion{"not_contains", "msg", "world"}); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("not_contains fail", func(t *testing.T) {
		if err := run(t, map[string]any{"msg": "hello world"}, nil, assertion{"not_contains", "msg", "world"}); err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("present pass", func(t *testing.T) {
		if err := run(t, map[string]any{"id": "abc"}, nil, assertion{"present", "id", ""}); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("present fail missing", func(t *testing.T) {
		if err := run(t, map[string]any{}, nil, assertion{"present", "id", ""}); err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("matches pass", func(t *testing.T) {
		if err := run(t, map[string]any{"date": "2026-07-14"}, nil, assertion{"matches", "date", `^\d{4}-\d{2}-\d{2}$`}); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("matches fail", func(t *testing.T) {
		if err := run(t, map[string]any{"date": "not-a-date"}, nil, assertion{"matches", "date", `^\d{4}-\d{2}-\d{2}$`}); err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("matches bad regex", func(t *testing.T) {
		if err := run(t, map[string]any{"x": "y"}, nil, assertion{"matches", "x", `[invalid`}); err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("length pass", func(t *testing.T) {
		if err := run(t, map[string]any{"items": []string{"a", "b", "c"}}, nil, assertion{"length", "items", "3"}); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("length fail wrong count", func(t *testing.T) {
		if err := run(t, map[string]any{"items": []string{"a"}}, nil, assertion{"length", "items", "3"}); err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("length fail not array", func(t *testing.T) {
		if err := run(t, map[string]any{"items": "not-array"}, nil, assertion{"length", "items", "1"}); err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("array_contains flat scalar array", func(t *testing.T) {
		if err := run(t, map[string]any{"tags": []string{"e2e", "inbox", "prod"}}, nil, assertion{"array_contains", "tags", "inbox"}); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("array_contains array of objects nested field", func(t *testing.T) {
		payload := map[string]any{
			"items": []any{
				map[string]any{"name": "Alice", "role": "admin"},
				map[string]any{"name": "Bob", "role": "user"},
			},
		}
		if err := run(t, payload, nil, assertion{"array_contains", "items.#.name", "Alice"}); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("array_contains doubly nested arrays", func(t *testing.T) {
		payload := map[string]any{
			"data": []any{
				map[string]any{
					"statuses": []any{
						map[string]any{"general_status": "requested"},
						map[string]any{"general_status": "sending"},
						map[string]any{"general_status": "not-sent"},
					},
				},
			},
		}
		if err := run(t, payload, nil, assertion{"array_contains", "data.#.statuses.#.general_status", "requested"}); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("array_contains fail value not present", func(t *testing.T) {
		if err := run(t, map[string]any{"tags": []string{"a", "b"}}, nil, assertion{"array_contains", "tags", "c"}); err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("array_contains fail field missing", func(t *testing.T) {
		if err := run(t, map[string]any{}, nil, assertion{"array_contains", "tags", "x"}); err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("map_contains dynamic keys hit", func(t *testing.T) {
		payload := map[string]any{
			"labels": map[string]any{"env": "prod", "tier": "web"},
		}
		if err := run(t, payload, nil, assertion{"map_contains", "labels.@values", "prod"}); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("map_contains fail value not present", func(t *testing.T) {
		payload := map[string]any{
			"labels": map[string]any{"env": "prod"},
		}
		if err := run(t, payload, nil, assertion{"map_contains", "labels.@values", "staging"}); err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("unknown type returns error", func(t *testing.T) {
		if err := run(t, map[string]any{"x": "y"}, nil, assertion{"bogus_type", "x", "y"}); err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("error message shows raw JSON not flat map", func(t *testing.T) {
		err := run(t, map[string]any{"items": []string{"a"}}, nil, assertion{"array_contains", "items", "missing"})
		if err == nil {
			t.Fatal("expected error")
		}
		msg := err.Error()
		if !strings.Contains(msg, "{") {
			t.Errorf("error should contain raw JSON opening brace, got: %s", msg)
		}
		if strings.Contains(msg, "__len__") {
			t.Errorf("error must not expose flat map internals, got: %s", msg)
		}
	})

	t.Run("variable substitution in value", func(t *testing.T) {
		vars := map[string]string{"expected_status": "active"}
		if err := run(t, map[string]any{"status": "active"}, vars, assertion{"equals", "status", "{{expected_status}}"}); err != nil {
			t.Fatal(err)
		}
	})
}

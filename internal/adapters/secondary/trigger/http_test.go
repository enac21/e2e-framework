package trigger

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	triggerasserts "e2e-framework/internal/adapters/secondary/assertions/trigger"
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

func TestRunResponseAssertions(t *testing.T) {
	type assertion struct {
		typ   string
		field string
		value string
	}

	reg := triggerasserts.NewDefaultTriggerAssertionRegistry()
	run := func(t *testing.T, payload any, vars map[string]string, a assertion) error {
		t.Helper()
		raw := mustJSON(payload)
		var m map[string]any
		_ = json.Unmarshal(raw, &m)
		flat := httputil.FlattenJSON(m)
		return reg.Run(
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

	t.Run("int_eq pass", func(t *testing.T) {
		if err := run(t, map[string]any{"count": 10}, nil, assertion{"int_eq", "count", "10"}); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("int_eq fail", func(t *testing.T) {
		err := run(t, map[string]any{"count": 10}, nil, assertion{"int_eq", "count", "11"})
		if err == nil {
			t.Fatal("expected error")
		}

		if !errors.Is(err, domain.ErrTriggerFailed) {
			t.Fatalf("want ErrTriggerFailed, got %v", err)
		}
	})

	t.Run("int_gt numeric comparison", func(t *testing.T) {
		if err := run(t, map[string]any{"count": 10}, nil, assertion{"int_gt", "count", "9"}); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("int_gt fail", func(t *testing.T) {
		if err := run(t, map[string]any{"count": 9}, nil, assertion{"int_gt", "count", "10"}); err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("int_gte pass equal", func(t *testing.T) {
		if err := run(t, map[string]any{"count": 10}, nil, assertion{"int_gte", "count", "10"}); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("int_gte fail", func(t *testing.T) {
		if err := run(t, map[string]any{"count": 9}, nil, assertion{"int_gte", "count", "10"}); err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("int_lt pass", func(t *testing.T) {
		if err := run(t, map[string]any{"count": 8}, nil, assertion{"int_lt", "count", "9"}); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("int_lt fail", func(t *testing.T) {
		if err := run(t, map[string]any{"count": 10}, nil, assertion{"int_lt", "count", "9"}); err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("int_lte pass equal", func(t *testing.T) {
		if err := run(t, map[string]any{"count": 9}, nil, assertion{"int_lte", "count", "9"}); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("int_lte fail", func(t *testing.T) {
		if err := run(t, map[string]any{"count": 10}, nil, assertion{"int_lte", "count", "9"}); err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("int fail actual not integer", func(t *testing.T) {
		if err := run(t, map[string]any{"count": "ten"}, nil, assertion{"int_eq", "count", "10"}); err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("int fail value not integer", func(t *testing.T) {
		if err := run(t, map[string]any{"count": 10}, nil, assertion{"int_eq", "count", "ten"}); err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("int variable substitution in value", func(t *testing.T) {
		vars := map[string]string{"expected_count": "10"}
		if err := run(t, map[string]any{"count": 10}, vars, assertion{"int_eq", "count", "{{expected_count}}"}); err != nil {
			t.Fatal(err)
		}
	})
}

func TestExecute_AbortsOnUnresolvedIncrement(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("request must not be sent when a placeholder stays unresolved")
	}))
	defer srv.Close()

	tr := NewHTTPTrigger(nil)
	vars := map[string]string{"broken": "abc"}

	t.Run("unresolved in url", func(t *testing.T) {
		_, err := tr.Execute(context.Background(), domain.TriggerConfig{
			URL: srv.URL + "/{{++(broken)}}",
		}, "run-1", vars)
		if !errors.Is(err, domain.ErrTriggerFailed) {
			t.Fatalf("want ErrTriggerFailed, got %v", err)
		}
	})

	t.Run("unresolved in body", func(t *testing.T) {
		_, err := tr.Execute(context.Background(), domain.TriggerConfig{
			URL:  srv.URL,
			Body: map[string]any{"counter": "{{++(broken)}}"},
		}, "run-1", vars)
		if !errors.Is(err, domain.ErrTriggerFailed) {
			t.Fatalf("want ErrTriggerFailed, got %v", err)
		}
	})
}

func TestExecute_IncrementCreatesMissingVar(t *testing.T) {
	gotPath := ""
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	tr := NewHTTPTrigger(nil)
	vars := map[string]string{}
	_, err := tr.Execute(context.Background(), domain.TriggerConfig{
		URL: srv.URL + "/{{++(fresh)}}",
	}, "run-1", vars)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if gotPath != "/1" {
		t.Fatalf("want path /1, got %s", gotPath)
	}

	if vars["fresh"] != "1" {
		t.Fatalf("expected vars[fresh]=1, got %q", vars["fresh"])
	}
}

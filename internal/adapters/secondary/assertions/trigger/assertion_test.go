package trigger

import (
	"testing"

	"github.com/tidwall/gjson"
)

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

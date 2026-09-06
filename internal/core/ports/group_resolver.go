package ports

import "time"

// TestGroup is a named, pre-configured list of tests resolved from the
// service configuration.
type TestGroup struct {
	Tests        []string
	TestDelay    time.Duration
	SkipFailTest bool
}

// GroupResolver resolves a named test group from configuration. The API
// adapter consumes this interface to stay decoupled from config internals.
type GroupResolver interface {
	Resolve(name string) (TestGroup, bool)
}

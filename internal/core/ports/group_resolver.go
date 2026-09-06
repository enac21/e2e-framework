package ports

import "time"

type TestGroup struct {
	Tests        []string
	TestDelay    time.Duration
	SkipFailTest bool
}

type GroupResolver interface {
	Resolve(name string) (TestGroup, bool)
}

package services

import (
	"e2e-framework/internal/core/ports"
	"e2e-framework/internal/pkg/config"
)

// GroupResolver implements ports.GroupResolver backed by the configured
// test_groups map.
type GroupResolver struct {
	groups map[string]config.TestGroupConfig
}

func NewGroupResolver(groups map[string]config.TestGroupConfig) *GroupResolver {
	return &GroupResolver{groups: groups}
}

func (r *GroupResolver) Resolve(name string) (ports.TestGroup, bool) {
	group, ok := r.groups[name]
	if !ok {
		return ports.TestGroup{}, false
	}

	return ports.TestGroup{
		Tests:        group.Tests,
		TestDelay:    group.TestDelay,
		SkipFailTest: group.SkipFailTest,
	}, true
}

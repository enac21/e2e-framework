package generators

import "github.com/google/uuid"

type UUIDGenerator struct{}

func init() {
	Register(UUIDGenerator{})
}

func (UUIDGenerator) Name() string {
	return "uuid"
}

func (UUIDGenerator) Generate(args string) (string, bool) {
	if args != "" {
		return "", false
	}

	return uuid.NewString(), true
}

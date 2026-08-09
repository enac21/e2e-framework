package generators

import (
	"math/rand/v2"
	"strconv"
	"strings"
)

type RandomIntGenerator struct{}

func init() {
	Register(RandomIntGenerator{})
}

func (RandomIntGenerator) Name() string {
	return "randomInt"
}

func (RandomIntGenerator) Generate(args string) (string, bool) {
	n, err := strconv.Atoi(strings.TrimSpace(args))
	if err != nil || n <= 0 {
		return "", false
	}

	max := 1
	for range n {
		max *= 10
	}

	return strconv.Itoa(rand.IntN(max)), true
}

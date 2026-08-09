package generators

import "testing"

func TestResolve_UnknownName(t *testing.T) {
	if result, ok := Resolve("doesNotExist", ""); ok || result != "" {
		t.Errorf("expected (\"\", false), got (%q, %v)", result, ok)
	}
}

func TestRegister_Overrides(t *testing.T) {
	Register(testGenerator{name: "override", value: "first"})
	Register(testGenerator{name: "override", value: "second"})

	result, ok := Resolve("override", "")
	if !ok || result != "second" {
		t.Errorf("expected registered generator to override, got (%q, %v)", result, ok)
	}
}

func TestRegister_NilIgnored(t *testing.T) {
	Register(nil)

	if _, ok := Resolve("nilSafe", ""); ok {
		t.Error("nil registration should not register anything")
	}
}

func TestResolve_RejectedArgs(t *testing.T) {
	Register(testGenerator{name: "onlyEmpty", value: "only-empty"})

	result, ok := Resolve("onlyEmpty", "something")
	if ok || result != "" {
		t.Errorf("expected rejected args to return (\"\", false), got (%q, %v)", result, ok)
	}
}

func TestResolve_RegisteredGeneratorsAvailable(t *testing.T) {
	if _, ok := Resolve("uuid", ""); !ok {
		t.Error("expected built-in uuid generator to be registered via init")
	}

	if _, ok := Resolve("randomInt", "3"); !ok {
		t.Error("expected built-in randomInt generator to be registered via init")
	}
}

type testGenerator struct {
	name  string
	value string
}

func (g testGenerator) Name() string {
	return g.name
}

func (g testGenerator) Generate(args string) (string, bool) {
	if args != "" {
		return "", false
	}

	return g.value, true
}

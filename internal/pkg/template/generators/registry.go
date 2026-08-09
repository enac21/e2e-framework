package generators

var registry = make(map[string]Generator)

func Register(g Generator) {
	if g == nil {
		return
	}

	registry[g.Name()] = g
}

func Resolve(name, args string) (result string, ok bool) {
	g, exists := registry[name]
	if !exists {
		return "", false
	}

	return g.Generate(args)
}

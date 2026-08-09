package generators

type Generator interface {
	Name() string
	Generate(args string) (result string, ok bool)
}

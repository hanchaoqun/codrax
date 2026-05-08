package service

// GreetServiceImpl is the canonical implementer of UserService.
// It is the only struct implementing both Greet and Goodbye in
// the multirepo-basic fixture.
type GreetServiceImpl struct {
	prefix string
}

// NewGreetServiceImpl constructs a greeter with the given prefix.
func NewGreetServiceImpl(prefix string) *GreetServiceImpl {
	return &GreetServiceImpl{prefix: prefix}
}

// Greet returns a greeting for the given name.
func (g *GreetServiceImpl) Greet(name string) string {
	return g.prefix + ", " + name
}

// Goodbye returns a farewell for the given name.
func (g *GreetServiceImpl) Goodbye(name string) string {
	return "Goodbye, " + name
}

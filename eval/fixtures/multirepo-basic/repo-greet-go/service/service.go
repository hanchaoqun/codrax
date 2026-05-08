package service

// UserService is the contract every greeter must implement.
// The fixture exposes one concrete implementer (GreetServiceImpl)
// in this same sub-repo so cross-sub-repo Implementers fan-out
// is exercised when the question is asked from the multi-repo
// parent.
type UserService interface {
	Greet(name string) string
	Goodbye(name string) string
}

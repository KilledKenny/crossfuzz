package runner

// Runner manages a target process and executes fuzz inputs against it.
type Runner interface {
	// Name returns the target name.
	Name() string
	// Start launches the target process.
	Start() error
	// Execute sends an input and returns the output and coverage bitmap.
	Execute(input []byte) (output []byte, coverage []byte, err error)
	// Stop terminates the target process and cleans up resources.
	Stop() error
}

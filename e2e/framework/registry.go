package framework

import "sort"

// Test is one registered e2e case.
type Test struct {
	// Name is the dotted identifier shown in output (e.g. "cli.MaxMemory").
	Name string
	// Tags are short labels used by the -tag filter (e.g. "harness", "parallel",
	// "comparer", "go").
	Tags []string
	// Func is the test body.
	Func func(t *T)
}

var registry []Test

// Register adds a test to the global registry. Called from package init().
func Register(t Test) {
	if t.Name == "" {
		panic("framework.Register: empty Name")
	}
	if t.Func == nil {
		panic("framework.Register: nil Func for " + t.Name)
	}
	registry = append(registry, t)
}

// Tests returns all registered tests, sorted by Name. Returns a copy so the
// caller may mutate the slice without affecting the registry.
func Tests() []Test {
	out := make([]Test, len(registry))
	copy(out, registry)
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// HasTag reports whether t carries the given tag.
func (t Test) HasTag(tag string) bool {
	for _, x := range t.Tags {
		if x == tag {
			return true
		}
	}
	return false
}

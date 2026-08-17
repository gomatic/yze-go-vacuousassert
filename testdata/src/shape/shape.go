// Package shape is a package the fixture calls THROUGH, so a package-qualified
// call can be told apart from a method call on a receiver.
package shape

// Index is called twice with different arguments in the fixture.
func Index(haystack, needle string) int { return len(haystack) + len(needle) }

package corpus

import "fmt"

// TB is the subset of *testing.T this package needs. Declaring it structurally
// keeps the "testing" import out of a non-test package.
type TB interface {
	Helper()
	Errorf(format string, args ...interface{})
	Logf(format string, args ...interface{})
}

// Check runs one compatibility assertion under the XFAIL registry.
//
// The four outcomes:
//
//	not registered, passes -> silent success
//	not registered, fails  -> normal test failure
//	registered,     fails  -> XFAIL, logged, build stays green
//	registered,     passes -> XPASS, build FAILS
//
// The last case is the point of the whole mechanism: fixing a protocol bug
// breaks the build until its entry is deleted from known_failures.json, so the
// registry cannot silently rot into a list of things that were fixed years ago.
//
// Assertions return an error rather than calling t.Errorf directly so that a
// failure can be captured and reinterpreted rather than immediately recorded.
func Check(t TB, id string, assert func() error) {
	t.Helper()

	err := assert()
	failure, registered := LookupKnownFailure(id)

	switch {
	case err == nil && !registered:
		// Passing as expected.

	case err != nil && !registered:
		t.Errorf("%s: %v", id, err)

	case err != nil && registered:
		t.Logf("XFAIL %s: %v\n      known bug: %s\n      tracked:   %s",
			id, err, failure.Reason, failure.TODO)

	case err == nil && registered:
		t.Errorf(
			"XPASS %s: this assertion is registered as a known failure but now PASSES.\n"+
				"      Delete its entry from internal/corpus/data/known_failures.json.\n"+
				"      Registered reason: %s\n"+
				"      Tracked at:        %s",
			id, failure.Reason, failure.TODO)
	}
}

// Errorf builds an assertion error. Sugar so assertion bodies read as
// `return corpus.Errorf(...)` rather than importing fmt everywhere.
func Errorf(format string, args ...interface{}) error {
	return fmt.Errorf(format, args...)
}

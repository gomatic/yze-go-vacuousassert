package vacuousassert_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"golang.org/x/tools/go/analysis/analysistest"

	vacuousassert "github.com/gomatic/yze-go-vacuousassert"
)

// TestAnAssertionComparedWithItselfIsReported drives the analyzer over a fixture
// holding every shape: the sentinel built inside the assertion, the bare
// repetition, a chain of method calls rooting to one subject, the fix that
// asserts against what the code returns, two different fields of one value, the
// one-sided assertions that have no two sides, and a comparison testify does not
// own.
func TestAnAssertionComparedWithItselfIsReported(t *testing.T) {
	analysistest.Run(t, analysistest.TestData(), vacuousassert.Analyzer, "a")
}

// TestRegistrationIsWellFormed pins the identifiers the suite keys on.
func TestRegistrationIsWellFormed(t *testing.T) {
	assert.NoError(t, vacuousassert.Registration.Validate())
	assert.Equal(t, "yze/vacuousassert", vacuousassert.Registration.RuleID())
	assert.Same(t, vacuousassert.Analyzer, vacuousassert.Registration.Analyzer)
}

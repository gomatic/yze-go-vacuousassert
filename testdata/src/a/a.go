package a

import (
	"assert"
	"require"
	"shape"
)

type Const string

func (c Const) Error() string          { return string(c) }
func (c Const) With(cause error) error { return c }
func (c Const) Wrapped() Const        { return c }

const ErrThing Const = "thing failed"

func produce() error { return ErrThing }

func Cases(t assert.T, r require.T) {
	// The shape that cost three analyzers their only test of a sentinel: the
	// expected value is built inside the assertion out of the constant it is
	// compared against.
	assert.ErrorIs(t, ErrThing.With(nil), ErrThing) // want `compares ErrThing with itself`
	require.ErrorIs(r, ErrThing.With(nil), ErrThing) // want `compares ErrThing with itself`

	// The same thing with no method call at all.
	assert.Equal(t, ErrThing, ErrThing) // want `compares ErrThing with itself`

	// Chained methods root to the same subject, and so do parentheses.
	assert.ErrorIs(t, ErrThing.Wrapped().With(nil), ErrThing) // want `compares ErrThing with itself`
	assert.ErrorIs(t, (ErrThing).With(nil), ErrThing)         // want `compares ErrThing with itself`

	// A TYPE ASSERTION is not stripped: `v.(Foo)` and `v.(Bar)` are two
	// different values of one variable, so rooting through it would report an
	// assertion that compares two genuinely different things.
	var any1 any = ErrThing
	assert.Equal(t, any1.(Const), any1.(error))

	// The FIX: assert against what the code returns.
	assert.ErrorIs(t, produce(), ErrThing)
	require.ErrorIs(r, produce(), ErrThing)

	// Two different fields of one value are two different sides.
	type pair struct{ left, right string }
	want := pair{}
	assert.Equal(t, want.left, want.right)

	// One-sided assertions have no two sides to be the same.
	assert.True(t, ErrThing != "")
	assert.NoError(t, nil)


	// A PACKAGE-QUALIFIED CALL is not a method call. Both sides reach into the
	// same package and are otherwise unrelated; rooting through the qualifier
	// would collapse them to the bare word `shape` and report them as one
	// expression. This was 55% of the rule's fleet findings.
	assert.Equal(t, shape.Index("a", "b"), shape.Index("c", "d"))

	// TWO SIBLINGS reached from one receiver are two different projections, not
	// one expression built from the other. Both root to `v`, and comparing roots
	// symmetrically reported them.
	v := ErrThing
	assert.Equal(t, v.Error(), string(v.Wrapped()))

	// The DIRECTIONAL test still catches the founding defect in both orders:
	// one side strips to the other AS WRITTEN.
	assert.ErrorIs(t, ErrThing, ErrThing.With(nil)) // want `compares ErrThing with itself`

	// A comparison that is not testify's is not this rule's business.
	_ = same(ErrThing, ErrThing)
}

func same(a, b Const) bool { return a == b }

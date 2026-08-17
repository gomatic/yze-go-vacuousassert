package vacuousassert

import (
	"go/ast"
	"go/importer"
	"go/parser"
	"go/token"
	"go/types"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// checked type-checks one expression in a package that imports strings, and
// returns the expression with the type information the analyzer would receive.
//
// The information is REAL rather than a stub, because the whole question these
// tests decide is one only the type checker can answer: whether the identifier
// on the left of a selector names a package or a value. A hand-built types.Info
// would let the test agree with whatever the code already does.
func checked(t *testing.T, expression string) (ast.Expr, *types.Info, *token.FileSet) {
	t.Helper()

	source := "package p\n\nimport \"strings\"\n\ntype box struct{ s string }\n\n" +
		"func (b box) Get() string { return b.s }\n\n" +
		"func mk() box { return box{} }\n\n" +
		"var _ = strings.Compare\n\n" +
		"var _ = " + expression + "\n"

	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "p.go", source, 0)
	require.NoError(t, err)

	info := &types.Info{Uses: map[*ast.Ident]types.Object{}, Defs: map[*ast.Ident]types.Object{}}
	config := types.Config{Importer: importer.Default()}
	_, err = config.Check("p", fset, []*ast.File{file}, info)
	require.NoError(t, err, "the probe source must type-check, or the test proves nothing")

	value := file.Decls[len(file.Decls)-1].(*ast.GenDecl).Specs[0].(*ast.ValueSpec)
	require.Len(t, value.Values, 1)

	return value.Values[0], info, fset
}

// TestRootedNeverStripsAPackageQualifiedCall names the invariant rooted's doc
// comment states: a package-qualified call is not a method call and is NEVER
// stripped. `strings.Index(a, b)` has no receiver under it, so rooting through
// the qualifier would yield the bare word `strings` and make every call into
// that package compare equal to every other — 55% of this rule's fleet findings.
func TestRootedNeverStripsAPackageQualifiedCall(t *testing.T) {
	expr, info, fset := checked(t, `strings.Index("a", "b")`)

	assert.Equal(t, `strings.Index("a", "b")`, rooted(info, fset, expr),
		"a package-qualified call roots to itself, never to the package name")
}

// TestRootedStripsAMethodCallOnAReceiver is the other side of the same
// boundary, and it is what stops the fix above from being a blanket refusal:
// the shape the rule exists for must still root.
func TestRootedStripsAMethodCallOnAReceiver(t *testing.T) {
	expr, info, fset := checked(t, `mk().Get()`)

	assert.Equal(t, "mk()", rooted(info, fset, expr),
		"a method reached through a value still roots to that value")
}

// TestRootedStepsOverEveryParenthesis pins the loop rather than a single step.
func TestRootedStepsOverEveryParenthesis(t *testing.T) {
	expr, info, fset := checked(t, `(((mk().Get())))`)

	assert.Equal(t, "mk()", rooted(info, fset, expr))
}

// TestRootsThroughReceiverAlwaysHoldsForANonIdentifierLeftSide names the
// invariant rootsThroughReceiver's doc comment states with "always": a selector
// whose left side is not an identifier is reaching through an expression, so it
// ALWAYS has a receiver and there is no package it could be naming.
func TestRootsThroughReceiverAlwaysHoldsForANonIdentifierLeftSide(t *testing.T) {
	expr, info, _ := checked(t, `mk().Get()`)
	selector := expr.(*ast.CallExpr).Fun.(*ast.SelectorExpr)
	require.IsType(t, &ast.CallExpr{}, selector.X, "the left side must not be an identifier")

	assert.True(t, rootsThroughReceiver(info, selector))
}

// TestRootsThroughReceiverRefusesWhatItCannotResolve pins the direction of the
// fail-safe, which is the opposite of the obvious one. Stripping is what makes
// two sides equal, so a wrong strip INVENTS a finding while a missed strip only
// withholds one. Absent type information, and an identifier the checker did not
// resolve, must therefore both answer false.
func TestRootsThroughReceiverRefusesWhatItCannotResolve(t *testing.T) {
	expr, info, _ := checked(t, `strings.Index("a", "b")`)
	selector := expr.(*ast.CallExpr).Fun.(*ast.SelectorExpr)

	assert.False(t, rootsThroughReceiver(nil, selector), "no type information is not a licence to strip")
	assert.False(t, rootsThroughReceiver(&types.Info{}, selector), "an unresolved identifier is not a receiver")
	assert.False(t, rootsThroughReceiver(info, selector), "a package qualifier is not a receiver")
}

// TestBuiltFromTheOtherIsDirectional is the contract that separates the defect
// from its lookalike. One side stripping to the OTHER AS WRITTEN is vacuous;
// two siblings reached from one value are not, however equal their roots.
func TestBuiltFromTheOtherIsDirectional(t *testing.T) {
	for name, probe := range map[string]struct {
		left, right string
		subject     string
		vacuous     bool
	}{
		"built from the other":          {`mk().Get()`, `mk()`, "mk()", true},
		"built from the other, swapped": {`mk()`, `mk().Get()`, "mk()", true},
		"identical expressions":         {`mk()`, `mk()`, "mk()", true},
		"two siblings of one value":     {`mk().Get()`, `mk().s`, "", false},
		"unrelated package calls":       {`strings.Index("a", "b")`, `strings.Index("c", "d")`, "", false},
	} {
		t.Run(name, func(t *testing.T) {
			left, info, fset := checked(t, probe.left)
			right, _, _ := checked(t, probe.right)

			subject, vacuous := builtFromTheOther(info, fset, left, right)

			assert.Equal(t, probe.vacuous, vacuous)
			assert.Equal(t, probe.subject, subject)
		})
	}
}

// TestBuiltFromTheOtherSaysNothingAboutAnExpressionItCannotRender pins the
// unprintable path: a rule that cannot read its subject reports nothing about
// it, rather than comparing two empty strings and calling them equal.
func TestBuiltFromTheOtherSaysNothingAboutAnExpressionItCannotRender(t *testing.T) {
	fset := token.NewFileSet()
	printable, info, _ := checked(t, `mk()`)

	// A nil expression is what go/printer actually refuses; an ast.BadExpr
	// prints as the literal text "BadExpr", which is why the unreadable side is
	// pinned with the input the printer really rejects rather than the one that
	// merely looks malformed.
	require.Empty(t, render(fset, nil), "the printer must refuse this, or the case proves nothing")

	subject, vacuous := builtFromTheOther(info, fset, nil, printable)

	assert.False(t, vacuous, "an unreadable side is not compared, so it is not a finding")
	assert.Empty(t, subject)
}

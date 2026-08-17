// Package vacuousassert provides a go/analysis analyzer that forbids an
// assertion whose two sides are the same expression.
//
// `assert.ErrorIs(t, ErrThing.With(nil), ErrThing)` reads like a test of the
// package's error and is a test of the error HELPER: the expected value is built
// inside the assertion out of the very constant it is compared against, so it
// holds whatever the code under test does. Replacing the sentinel the code
// actually returns with any other leaves the suite green — which is exactly what
// happened, in three analyzers, to the one assertion each had for its own
// sentinel.
//
// The same shape covers `assert.Equal(t, want, want)` and any assertion whose
// sides differ only by a method call. The fix is to call the code: assert
// against what the function under test RETURNS, not against a value the test
// constructed a line earlier from the same constant.
package vacuousassert

import (
	"go/ast"
	"go/printer"
	"go/token"
	"go/types"
	"strings"

	goyze "github.com/gomatic/go-yze"
	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/inspect"
	"golang.org/x/tools/go/ast/inspector"
)

const message = "this assertion compares %s with itself, so it holds whatever the code under test does; " +
	"assert against the value the code RETURNS"

// comparisons are the testify assertions whose second and third arguments are
// the two sides of one comparison. An assertion taking one value — Nil, True,
// Empty — has no two sides to be the same.
var comparisons = map[string]bool{
	"Equal": true, "NotEqual": true, "EqualValues": true, "Same": true, "NotSame": true,
	"ErrorIs": true, "NotErrorIs": true, "ErrorAs": true, "Contains": true, "NotContains": true,
	"Greater": true, "Less": true, "GreaterOrEqual": true, "LessOrEqual": true,
}

// assertionPackages are the testify entry points this rule reads.
var assertionPackages = map[string]bool{"assert": true, "require": true}

// Analyzer reports an assertion whose two sides are one expression.
var Analyzer = &analysis.Analyzer{
	Name:     "vacuousassert",
	Doc:      "reports a testify assertion comparing an expression with itself",
	Requires: []*analysis.Analyzer{inspect.Analyzer},
	Run:      run,
}

// Registration declares this analyzer to the yze framework.
var Registration = goyze.Registration{
	Name:       "vacuousassert",
	Categories: []goyze.Category{"tests"},
	URL:        "https://gomatic.github.io/docs.yze/",
	Analyzer:   Analyzer,
}

// run reports each assertion whose two sides are the same expression.
func run(pass *analysis.Pass) (any, error) {
	insp := pass.ResultOf[inspect.Analyzer].(*inspector.Inspector)
	insp.Preorder([]ast.Node{(*ast.CallExpr)(nil)}, func(node ast.Node) {
		call := node.(*ast.CallExpr)
		if !isComparison(call) || len(call.Args) < 3 {
			return
		}
		if subject, vacuous := builtFromTheOther(pass.TypesInfo, pass.Fset, call.Args[1], call.Args[2]); vacuous {
			pass.Reportf(call.Pos(), message, subject)
		}
	})
	return nil, nil
}

// isComparison reports a testify call whose next two arguments are the two sides
// of one comparison.
func isComparison(call *ast.CallExpr) bool {
	selector, isSelector := call.Fun.(*ast.SelectorExpr)
	if !isSelector {
		return false
	}
	pkg, isIdent := selector.X.(*ast.Ident)
	return isIdent && assertionPackages[pkg.Name] && comparisons[selector.Sel.Name]
}

// builtFromTheOther reports whether one side of the assertion was CONSTRUCTED
// OUT OF the other, and names the side it was built from.
//
// That is the defect, and it is not the same question as "do both sides root to
// the same place". An assertion is vacuous when the expected value is derived
// from the very thing it is compared against — `ErrThing.With(nil)` against
// `ErrThing` — because then it holds whatever the code under test does. Two
// SIBLINGS reached from one value are not that: `v.Left()` and `v.Right()` both
// root to `v`, and comparing them is an ordinary, falsifiable assertion about
// two different projections.
//
// Comparing roots symmetrically confused the two and reported every sibling
// pair. Across the fleet that, together with the package-qualified case in
// [rooted], was 40 of 49 findings. So the test is directional: a side is
// vacuous when stripping its method calls yields the OTHER SIDE AS WRITTEN.
// Identical expressions satisfy it in both directions and are still reported.
func builtFromTheOther(info *types.Info, fset *token.FileSet, left, right ast.Expr) (string, bool) {
	leftText, rightText := render(fset, left), render(fset, right)
	if leftText == "" || rightText == "" {
		return "", false
	}
	if rooted(info, fset, left) == rightText {
		return rightText, true
	}
	if rooted(info, fset, right) == leftText {
		return leftText, true
	}

	return "", false
}

// rooted is an expression with its trailing METHOD CALLS removed, so one side of
// an assertion can be tested for having been built out of the other.
//
// `ErrThing.With(nil)` roots to `ErrThing`, which is what makes it the same side
// as a bare `ErrThing`. A call on something else — `report(nil)`, `f(x).Field` —
// roots to itself and matches nothing, which is the point: an assertion against
// what the code RETURNS is exactly what this rule is asking for.
//
// A TYPE ASSERTION is deliberately not stripped. `v.(Foo)` and `v.(Bar)` are two
// different values of one variable, so rooting through one would report an
// assertion comparing two genuinely different things.
//
// A PACKAGE-QUALIFIED CALL IS NOT A METHOD CALL AND IS NEVER STRIPPED.
// `strings.Index(a, b)` selects Index from the package strings, so there is no
// receiver under it; stripping it would root the call to the bare word
// `strings`, and every call into that package would then root to the same
// string and compare equal to every other. That is not a corner case: it was
// 55% of this rule's findings across the fleet, comparing two genuinely
// different calls and calling them one expression.
func rooted(info *types.Info, fset *token.FileSet, expr ast.Expr) string {
	for {
		switch typed := expr.(type) {
		case *ast.ParenExpr:
			expr = typed.X
		case *ast.CallExpr:
			method, isMethod := typed.Fun.(*ast.SelectorExpr)
			if !isMethod || !rootsThroughReceiver(info, method) {
				return render(fset, expr)
			}
			expr = method.X
		default:
			return render(fset, expr)
		}
	}
}

// rootsThroughReceiver reports whether the selector reaches a method through a
// VALUE, which is the only shape rooting is for.
//
// A selector whose left side is not an identifier — `f(x).Method()`,
// `a.b.Method()` — is reaching through an expression, so it always has a
// receiver. An identifier is the ambiguous case: it is a receiver when it names
// a value and a package qualifier when it names an import, and only the type
// checker can tell them apart.
//
// An identifier the type checker cannot resolve is treated as NOT a receiver,
// which is the direction that fails safe. Stripping is what makes two sides
// equal, so a wrong strip invents a finding while a missed strip only withholds
// one, and this rule's cost has been false positives rather than silence.
func rootsThroughReceiver(info *types.Info, selector *ast.SelectorExpr) bool {
	identifier, isIdentifier := selector.X.(*ast.Ident)
	if !isIdentifier {
		return true
	}
	if info == nil {
		return false
	}
	used := info.Uses[identifier]
	if used == nil {
		return false
	}
	_, isPackage := used.(*types.PkgName)

	return !isPackage
}

// render is an expression as it was written, for comparing two of them and for
// naming one in a message.
func render(fset *token.FileSet, expr ast.Expr) string {
	var text strings.Builder
	if err := printer.Fprint(&text, fset, expr); err != nil {
		// Unprintable expressions compare equal to nothing, which reports no
		// finding — a rule that cannot read its subject says nothing about it.
		return ""
	}
	return text.String()
}

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
		subject := rooted(pass.Fset, call.Args[1])
		if subject != "" && subject == rooted(pass.Fset, call.Args[2]) {
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

// rooted is an expression with its trailing METHOD CALLS removed, so the two
// sides of an assertion can be compared for being the same thing.
//
// `ErrThing.With(nil)` roots to `ErrThing`, which is what makes it the same side
// as a bare `ErrThing`. A call on something else — `report(nil)`, `f(x).Field` —
// roots to itself and matches nothing, which is the point: an assertion against
// what the code RETURNS is exactly what this rule is asking for.
//
// A TYPE ASSERTION is deliberately not stripped. `v.(Foo)` and `v.(Bar)` are two
// different values of one variable, so rooting through one would report an
// assertion comparing two genuinely different things.
func rooted(fset *token.FileSet, expr ast.Expr) string {
	for {
		switch typed := expr.(type) {
		case *ast.ParenExpr:
			expr = typed.X
		case *ast.CallExpr:
			method, isMethod := typed.Fun.(*ast.SelectorExpr)
			if !isMethod {
				return render(fset, expr)
			}
			expr = method.X
		default:
			return render(fset, expr)
		}
	}
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

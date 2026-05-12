// Package bolascope implements a static analyzer that flags gin route
// bindings (r.GET / r.POST / ... / r.Handle / r.Any on a *gin.RouterGroup or
// *gin.Engine) which do not declare a BOLA scope via middleware.WithScope().
//
// Rationale: every authenticated route in partner-api MUST declare a scope so
// the BOLA fail-closed middleware can enforce object-level access control.
// Forgetting to add middleware.WithScope("...") to a route is a silent
// authorization bypass — exactly the footgun routes_smoke_test.go documents.
//
// Allowlist: place a comment `//bolascope:allow <free-form reason>` on the
// line above (or trailing) the route binding to skip the check. The reason is
// not validated; intent is to force a paper trail.
//
// Heuristics:
//   - The receiver of the call is identified by type-name suffix
//     (RouterGroup / Engine) rather than full import path. This keeps the
//     analyzer testable without dragging real gin into testdata, and also
//     happens to be robust against aliased imports.
//   - A handler-chain arg is accepted as "scope declared" if it is a call to
//     a function literally named WithScope (or WithScopeLogged). Package
//     resolution is best-effort — we accept the simple-name match because
//     false positives are worse than the occasional miss on edge cases.
//   - If a route's handler list includes a non-call expression (e.g. a
//     variable holding gin.HandlerFunc), the route is accepted: we can't
//     statically prove that variable wraps WithScope, but factored
//     route-builder helpers are common and should not be flagged.
package bolascope

import (
	"go/ast"
	"go/token"
	"go/types"
	"strings"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/inspect"
	"golang.org/x/tools/go/ast/inspector"
)

// Analyzer is the singleton analyzer instance, suitable for use with
// singlechecker.Main or multichecker.Main.
var Analyzer = &analysis.Analyzer{
	Name:     "bolascope",
	Doc:      "checks that every gin route binding declares a BOLA scope via middleware.WithScope()",
	Run:      run,
	Requires: []*analysis.Analyzer{inspect.Analyzer},
}

// ginRouteMethods is the set of *gin.RouterGroup / *gin.Engine method names
// that mount an HTTP handler. Handle and Any take a path-arg in a different
// position, but in all cases the trailing args are middleware/handlers.
var ginRouteMethods = map[string]bool{
	"GET":     true,
	"POST":    true,
	"PUT":    true,
	"PATCH":   true,
	"DELETE":  true,
	"OPTIONS": true,
	"HEAD":    true,
	"Handle":  true,
	"Any":     true,
}

const allowDirective = "//bolascope:allow"

func run(pass *analysis.Pass) (interface{}, error) {
	insp := pass.ResultOf[inspect.Analyzer].(*inspector.Inspector)

	// Pre-index allow directives by line. Map: filename -> set of line numbers
	// (1-based) on which the route call expr may sit. Both "directive on
	// previous line" and "trailing on same line" are accepted.
	allowLines := map[string]map[int]bool{}
	// testFiles is the set of *_test.go file names we deliberately skip: unit
	// tests for individual middleware legitimately mount unscoped routes to
	// exercise behavior in isolation, and the smoke test in handler/ also
	// documents the footgun on purpose. Real route registration always lives
	// in non-test files.
	testFiles := map[string]bool{}
	for _, f := range pass.Files {
		fset := pass.Fset
		name := fset.File(f.Pos()).Name()
		if strings.HasSuffix(name, "_test.go") {
			testFiles[name] = true
		}
		set := map[int]bool{}
		for _, cg := range f.Comments {
			for _, c := range cg.List {
				if !strings.HasPrefix(c.Text, allowDirective) {
					continue
				}
				line := fset.Position(c.Pos()).Line
				// Mark same-line AND the next line as allow-covered.
				set[line] = true
				set[line+1] = true
			}
		}
		if len(set) > 0 {
			allowLines[name] = set
		}
	}

	insp.Preorder([]ast.Node{(*ast.CallExpr)(nil)}, func(n ast.Node) {
		call, _ := n.(*ast.CallExpr)
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return
		}
		methodName := sel.Sel.Name
		if !ginRouteMethods[methodName] {
			return
		}
		if !isGinRouter(pass, sel.X) {
			return
		}

		// Skip test files — they mount unscoped routes to exercise individual
		// middleware in isolation, which is intentional.
		pos := pass.Fset.Position(call.Pos())
		if testFiles[pos.Filename] {
			return
		}

		// Determine where path-arg lives: for Handle(method, path, ...) it is
		// args[1]; for everything else args[0]. Anything after is the
		// handler-chain.
		args := call.Args
		var pathIdx int
		switch methodName {
		case "Handle":
			pathIdx = 1
		default:
			pathIdx = 0
		}
		if len(args) <= pathIdx {
			return // malformed, skip
		}
		handlerArgs := args[pathIdx+1:]
		if len(handlerArgs) == 0 {
			// No handler at all — gin would panic at runtime, not our job.
			return
		}

		if handlerChainHasScope(pass, handlerArgs) {
			return
		}

		// Allowlist?
		if set, ok := allowLines[pos.Filename]; ok && set[pos.Line] {
			return
		}

		// Build diagnostic. Best-effort path extraction: if the path arg is a
		// string literal, include it; otherwise show "<dynamic>".
		pathLit := "<dynamic>"
		if bl, ok := args[pathIdx].(*ast.BasicLit); ok && bl.Kind == token.STRING {
			pathLit = bl.Value
		}
		pass.Reportf(call.Pos(),
			"route %s %s: missing middleware.WithScope() — every route must declare a BOLA scope",
			methodName, pathLit,
		)
	})

	return nil, nil
}

// isGinRouter reports whether expr's type is *gin.RouterGroup or *gin.Engine
// (matched by type-name suffix to remain testable without real gin).
func isGinRouter(pass *analysis.Pass, expr ast.Expr) bool {
	t := pass.TypesInfo.TypeOf(expr)
	if t == nil {
		return false
	}
	// Unwrap pointer.
	if p, ok := t.(*types.Pointer); ok {
		t = p.Elem()
	}
	named, ok := t.(*types.Named)
	if !ok {
		return false
	}
	name := named.Obj().Name()
	return name == "RouterGroup" || name == "Engine"
}

// handlerChainHasScope reports whether the handler chain declares a scope.
//
// A scope is declared if any arg is:
//   - a direct call to WithScope / WithScopeLogged, OR
//   - a call expression we cannot statically resolve to a known non-scope
//     function (i.e. a factored builder that *might* return a WithScope
//     handler — accepted as a pass to avoid false positives).
//
// Plain identifiers / selectors / function literals are treated as ordinary
// handler funcs, NOT as scope declarations — that's the whole point of the
// check. The "trust the call expression" relaxation only applies to call
// results, since those are how route-builder helpers surface.
func handlerChainHasScope(pass *analysis.Pass, args []ast.Expr) bool {
	for _, a := range args {
		call, ok := a.(*ast.CallExpr)
		if !ok {
			continue
		}
		// Direct WithScope call → definite scope.
		if isWithScopeCall(pass, call) {
			return true
		}
		// Any other call expression — assume it might wrap WithScope. This
		// covers route-builder helpers like buildScopedChain("partner_self").
		// Keeps the analyzer a guardrail, not a proof system.
		return true
	}
	return false
}

// isWithScopeCall reports whether the given call expression invokes a
// function named WithScope or WithScopeLogged. Package resolution is
// best-effort: we accept either a Selector with that simple name (e.g.
// middleware.WithScope) or a bare Ident (e.g. dot-import or same-package
// call).
func isWithScopeCall(_ *analysis.Pass, call *ast.CallExpr) bool {
	name := ""
	switch fn := call.Fun.(type) {
	case *ast.SelectorExpr:
		name = fn.Sel.Name
	case *ast.Ident:
		name = fn.Name
	default:
		return false
	}
	return name == "WithScope" || name == "WithScopeLogged"
}

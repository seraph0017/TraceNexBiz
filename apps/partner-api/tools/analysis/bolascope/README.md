# bolascope — static analyzer for missing `middleware.WithScope()`

This package implements a Go static analyzer (`golang.org/x/tools/go/analysis`)
that fails the build when a gin route binding does not declare a BOLA scope.

## What it does

For every call to `r.GET / r.POST / r.PUT / r.PATCH / r.DELETE / r.OPTIONS /
r.HEAD / r.Handle / r.Any` on a `*gin.RouterGroup` or `*gin.Engine`, the
analyzer checks the handler-chain args for at least one call to
`middleware.WithScope(...)` (or `WithScopeLogged`). Missing scope produces a
diagnostic:

```
route GET "/foo": missing middleware.WithScope() — every route must declare a BOLA scope
```

Without `WithScope`, the BOLA fail-closed middleware has no scope to enforce,
which silently turns the route into an authorization bypass. This is the
exact footgun documented in `internal/handler/routes_smoke_test.go`.

## Running it

```bash
# via Makefile
make lint-bolascope

# raw
go run ./tools/analysis/bolascope/cmd/bolascope ./...
```

Exit code 0 = clean, 3 = diagnostics emitted.

## Allowlisting a route

Place `//bolascope:allow <free-form reason>` either on the line directly above
the route binding, or trailing on the same line:

```go
//bolascope:allow public liveness probe, no actor identity
r.GET("/healthz", handler.Healthz)

r.GET("/legacy", legacyHandler) //bolascope:allow scheduled for removal in W2
```

The reason is recorded but not validated — its purpose is to force a paper
trail when bypassing the check.

## Known limitations

- **Factored route-builders are accepted as a pass.** If a handler arg is a
  call expression that the analyzer can't statically prove is *not* WithScope
  (e.g. `r.GET("/x", buildScopedChain("partner_self"))`), the route passes.
  False positives on indirection are worse than the occasional miss.
- **Plain identifiers / function literals do NOT satisfy scope.** Only call
  expressions count. `r.GET("/x", myHandler)` is flagged.
- **Test files (`*_test.go`) are skipped.** Unit tests for middleware
  legitimately mount unscoped routes to exercise behavior in isolation; the
  smoke test in `internal/handler/` documents the missing-scope footgun on
  purpose.
- **Receiver detection is by type-name suffix** (`RouterGroup` / `Engine`),
  not import path. This makes the analyzer trivially testable with a
  `fakegin` stub and is robust against aliased imports.
- **WithScope detection is by simple name**, not package-path resolution.
  Anything named `WithScope` or `WithScopeLogged` satisfies the check. This is
  a deliberate relaxation to keep the analyzer's logic simple.

## CI wiring

The analyzer is not yet wired into a golangci-lint plugin (their plugin API
churns enough that maintaining one is more cost than the analyzer itself).
Ops should add a step to the partner-api CI workflow:

```yaml
- name: Lint BOLA scopes
  run: make -C apps/partner-api lint-bolascope
```

## Layout

```
tools/analysis/bolascope/
├── analyzer.go              # Analyzer implementation
├── analyzer_test.go         # analysistest.Run() driver
├── cmd/bolascope/main.go    # singlechecker CLI entry point
├── README.md                # this file
└── testdata/src/
    ├── fakegin/             # minimal gin stub
    ├── pass/                # route with WithScope — no diagnostic
    ├── fail/                # route without WithScope — 2 diagnostics
    └── allow/               # route without WithScope but allowlisted — no diagnostic
```

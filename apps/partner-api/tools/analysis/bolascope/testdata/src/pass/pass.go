package pass

import "fakegin"

func dummy(*fakegin.Context) {}

// buildChain is a factored route-builder helper. Its return value is a call
// expression that the analyzer can't statically prove wraps WithScope — it
// accepts it as a pass.
func buildChain() fakegin.HandlerFunc { return dummy }

func Register(r *fakegin.RouterGroup) {
	r.GET("/ok", fakegin.WithScope("partner_self"), dummy)
	r.Handle("PUT", "/h", fakegin.WithScope("public"), dummy)
	r.POST("/factored", buildChain(), dummy)
}

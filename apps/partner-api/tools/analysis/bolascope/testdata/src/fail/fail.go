package fail

import "fakegin"

func dummy(*fakegin.Context) {}

func Register(r *fakegin.RouterGroup) {
	r.GET("/missing", dummy) // want `route GET "/missing": missing middleware.WithScope\(\) — every route must declare a BOLA scope`
	r.Handle("POST", "/also-missing", dummy) // want `route Handle "/also-missing": missing middleware.WithScope\(\) — every route must declare a BOLA scope`
}

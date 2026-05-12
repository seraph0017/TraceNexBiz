package allow

import "fakegin"

func dummy(*fakegin.Context) {}

func Register(r *fakegin.RouterGroup) {
	//bolascope:allow internal healthcheck, no actor identity to scope
	r.GET("/healthz", dummy)

	r.GET("/legacy", dummy) //bolascope:allow legacy debug endpoint, removed in W2
}

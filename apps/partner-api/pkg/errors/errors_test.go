package errors

import (
	"errors"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestWrapMapsHTTP(t *testing.T) {
	cases := []struct {
		code Code
		want int
	}{
		{CodeResNotFound, http.StatusNotFound},
		{CodeAuthJWTRevoked, http.StatusUnauthorized},
		{CodePermForbidden, http.StatusForbidden},
		{CodeIdemReusedDifferentBody, http.StatusConflict},
		{CodeFyAPI5xx, http.StatusBadGateway},
	}
	for _, c := range cases {
		t.Run(string(c.code), func(t *testing.T) {
			ae := Wrap(errors.New("x"), c.code)
			assert.Equal(t, c.want, ae.HTTP)
		})
	}
}

func TestAsAppErrorRoundTrip(t *testing.T) {
	ae := Wrap(errors.New("boom"), CodeWalletInsufficient)
	got, ok := AsAppError(ae)
	assert.True(t, ok)
	assert.Equal(t, CodeWalletInsufficient, got.Code)
}

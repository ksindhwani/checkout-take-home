//go:build integration

// This file exercises the real bank simulator started via `docker-compose up`.
// It is excluded from the default `go test ./...` run so the test suite stays
// fast and dependency-free; run it explicitly with:
//
//	docker-compose up -d bank_simulator
//	go test -tags=integration ./internal/bank/...
package bank

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHTTPClient_Authorize_AgainstRealSimulator(t *testing.T) {
	client := NewHTTPClient("http://localhost:8080", 5*time.Second)

	t.Run("odd last digit is authorized", func(t *testing.T) {
		req := testRequest()
		req.CardNumber = "2222405343248871"

		resp, err := client.Authorize(context.Background(), req)
		require.NoError(t, err)
		assert.True(t, resp.Authorized)
		assert.NotEmpty(t, resp.AuthorizationCode)
	})

	t.Run("even last digit is declined", func(t *testing.T) {
		req := testRequest()
		req.CardNumber = "2222405343248872"

		resp, err := client.Authorize(context.Background(), req)
		require.NoError(t, err)
		assert.False(t, resp.Authorized)
	})

	t.Run("last digit zero is unavailable", func(t *testing.T) {
		req := testRequest()
		req.CardNumber = "2222405343248870"

		_, err := client.Authorize(context.Background(), req)
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrUnavailable)
	})
}

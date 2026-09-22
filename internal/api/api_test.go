package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/cko-recruitment/payment-gateway-challenge-go/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A shutdown signal must not abort a payment that is already in flight:
// the server should stop accepting new requests but let this one finish.
func TestRun_ShutdownDrainsInFlightRequests(t *testing.T) {
	entered := make(chan struct{})
	bank := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(entered)
		time.Sleep(300 * time.Millisecond)
		_ = json.NewEncoder(w).Encode(map[string]any{"authorized": true, "authorization_code": "code"})
	}))
	defer bank.Close()

	l, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	addr := l.Addr().String()
	require.NoError(t, l.Close())

	a := New(config.Config{BankBaseURL: bank.URL, BankTimeout: 5 * time.Second, LogLevel: "error"})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	runErr := make(chan error, 1)
	go func() { runErr <- a.Run(ctx, addr) }()

	require.Eventually(t, func() bool {
		resp, err := http.Get("http://" + addr + "/ping")
		if err != nil {
			return false
		}
		resp.Body.Close()
		return resp.StatusCode == http.StatusOK
	}, 2*time.Second, 20*time.Millisecond)

	body, _ := json.Marshal(map[string]any{
		"card_number":  "2222405343248871",
		"expiry_month": 4,
		"expiry_year":  time.Now().Year() + 2,
		"currency":     "GBP",
		"amount":       100,
		"cvv":          "123",
	})

	status := make(chan int, 1)
	go func() {
		resp, err := http.Post("http://"+addr+"/api/payments", "application/json", bytes.NewReader(body))
		if err != nil {
			status <- 0
			return
		}
		resp.Body.Close()
		status <- resp.StatusCode
	}()

	<-entered // the request is now mid-flight, waiting on the bank
	cancel()  // simulate SIGTERM

	assert.Equal(t, http.StatusCreated, <-status, "in-flight payment should complete, not be aborted by shutdown")
	assert.NoError(t, <-runErr)
}

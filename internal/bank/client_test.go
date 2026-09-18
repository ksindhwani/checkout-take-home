package bank

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testRequest() AuthorizeRequest {
	return AuthorizeRequest{
		CardNumber:  "2222405343248877",
		ExpiryMonth: 4,
		ExpiryYear:  2025,
		Currency:    "GBP",
		Amount:      100,
		CVV:         "123",
	}
}

func TestHTTPClient_Authorize_Authorized(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "/payments", r.URL.Path)

		var got wireRequest
		require.NoError(t, json.NewDecoder(r.Body).Decode(&got))
		assert.Equal(t, "04/2025", got.ExpiryDate)

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(wireResponse{Authorized: true, AuthorizationCode: "auth-code-123"})
	}))
	defer server.Close()

	client := NewHTTPClient(server.URL, time.Second)
	resp, err := client.Authorize(context.Background(), testRequest())

	require.NoError(t, err)
	assert.True(t, resp.Authorized)
	assert.Equal(t, "auth-code-123", resp.AuthorizationCode)
}

func TestHTTPClient_Authorize_Declined(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(wireResponse{Authorized: false, AuthorizationCode: ""})
	}))
	defer server.Close()

	client := NewHTTPClient(server.URL, time.Second)
	resp, err := client.Authorize(context.Background(), testRequest())

	require.NoError(t, err)
	assert.False(t, resp.Authorized)
}

func TestHTTPClient_Authorize_BankServiceUnavailable(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()

	client := NewHTTPClient(server.URL, time.Second)
	_, err := client.Authorize(context.Background(), testRequest())

	require.Error(t, err)
	assert.ErrorIs(t, err, ErrUnavailable)
}

func TestHTTPClient_Authorize_Unreachable(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	server.Close() // closed immediately: connections to it will fail

	client := NewHTTPClient(server.URL, time.Second)
	_, err := client.Authorize(context.Background(), testRequest())

	require.Error(t, err)
	assert.ErrorIs(t, err, ErrUnavailable)
}

func TestHTTPClient_Authorize_Timeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(50 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := NewHTTPClient(server.URL, 5*time.Millisecond)
	_, err := client.Authorize(context.Background(), testRequest())

	require.Error(t, err)
	assert.ErrorIs(t, err, ErrUnavailable)
}

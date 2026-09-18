package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPingHandler(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/ping", nil)
	w := httptest.NewRecorder()

	PingHandler()(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	var got pong
	require.NoError(t, json.NewDecoder(w.Body).Decode(&got))
	assert.Equal(t, "pong", got.Message)
}

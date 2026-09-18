// Package bank implements a client for the acquiring bank (the Mountebank
// simulator described in the project README).
package bank

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"
)

// ErrUnavailable means the payment outcome is unknown: the bank could not be
// reached, timed out, or returned an error.
var ErrUnavailable = errors.New("bank unavailable")

// Client authorizes a payment against the acquiring bank.
type Client interface {
	Authorize(ctx context.Context, req AuthorizeRequest) (*AuthorizeResponse, error)
}

// HTTPClient is the production Client, calling the bank simulator over HTTP.
type HTTPClient struct {
	baseURL    string
	httpClient *http.Client
}

func NewHTTPClient(baseURL string, timeout time.Duration) *HTTPClient {
	return &HTTPClient{
		baseURL:    baseURL,
		httpClient: &http.Client{Timeout: timeout},
	}
}

func (c *HTTPClient) Authorize(ctx context.Context, req AuthorizeRequest) (*AuthorizeResponse, error) {
	payload, err := json.Marshal(wireRequest{
		CardNumber: req.CardNumber,
		ExpiryDate: fmt.Sprintf("%02d/%d", req.ExpiryMonth, req.ExpiryYear),
		Currency:   req.Currency,
		Amount:     req.Amount,
		Cvv:        req.CVV,
	})
	if err != nil {
		return nil, fmt.Errorf("bank: encode request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/payments", bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("bank: build request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusOK:
		var wireResp wireResponse
		if err := json.NewDecoder(resp.Body).Decode(&wireResp); err != nil {
			return nil, fmt.Errorf("bank: decode response: %w", err)
		}
		return &AuthorizeResponse{
			Authorized:        wireResp.Authorized,
			AuthorizationCode: wireResp.AuthorizationCode,
		}, nil
	default:
		// Any non-200 is treated as no decision, not as Declined.
		return nil, fmt.Errorf("%w: unexpected status %d", ErrUnavailable, resp.StatusCode)
	}
}

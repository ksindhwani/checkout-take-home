package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/cko-recruitment/payment-gateway-challenge-go/internal/bank"
	"github.com/cko-recruitment/payment-gateway-challenge-go/internal/models"
	"github.com/cko-recruitment/payment-gateway-challenge-go/internal/repository"
	"github.com/cko-recruitment/payment-gateway-challenge-go/internal/service"
	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// stubBankClient returns a fixed response/error and counts how many times
// it was called, so tests can assert an idempotent replay never re-calls
// the bank.
type stubBankClient struct {
	mu    sync.Mutex
	calls int
	resp  *bank.AuthorizeResponse
	err   error
}

func (s *stubBankClient) Authorize(_ context.Context, _ bank.AuthorizeRequest) (*bank.AuthorizeResponse, error) {
	s.mu.Lock()
	s.calls++
	s.mu.Unlock()
	return s.resp, s.err
}

func (s *stubBankClient) callCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.calls
}

// sequencedBankClient returns a different result on each successive call,
// used to simulate the bank failing once and then succeeding on retry.
type sequencedBankClient struct {
	mu      sync.Mutex
	i       int
	results []struct {
		resp *bank.AuthorizeResponse
		err  error
	}
}

func (s *sequencedBankClient) Authorize(_ context.Context, _ bank.AuthorizeRequest) (*bank.AuthorizeResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	r := s.results[s.i]
	if s.i < len(s.results)-1 {
		s.i++
	}
	return r.resp, r.err
}

// blockingBankClient blocks inside Authorize until release is closed, and
// signals via entered (once) that it has been called, so a test can
// deterministically fire a second, concurrent request while the first is
// still in flight.
type blockingBankClient struct {
	entered chan struct{}
	release chan struct{}
	once    sync.Once
	resp    *bank.AuthorizeResponse
}

func (b *blockingBankClient) Authorize(_ context.Context, _ bank.AuthorizeRequest) (*bank.AuthorizeResponse, error) {
	b.once.Do(func() { close(b.entered) })
	<-b.release
	return b.resp, nil
}

func newTestRouter(bankClient bank.Client) (*chi.Mux, *repository.PaymentsRepository) {
	repo := repository.NewPaymentsRepository()
	svc := service.NewPaymentService(repo, bankClient)
	h := NewPaymentsHandler(svc)

	r := chi.NewRouter()
	r.Post("/api/payments", h.PostHandler())
	r.Get("/api/payments/{id}", h.GetHandler())
	return r, repo
}

func validPaymentJSON() []byte {
	body, _ := json.Marshal(models.PostPaymentRequest{
		CardNumber:  "2222405343248877",
		ExpiryMonth: 4,
		ExpiryYear:  time.Now().Year() + 2,
		Currency:    "GBP",
		Amount:      100,
		Cvv:         "123",
	})
	return body
}

func postPayment(r *chi.Mux, body []byte, idempotencyKey string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/api/payments", bytes.NewReader(body))
	if idempotencyKey != "" {
		req.Header.Set("Idempotency-Key", idempotencyKey)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func TestPostHandler_Authorized(t *testing.T) {
	r, _ := newTestRouter(&stubBankClient{resp: &bank.AuthorizeResponse{Authorized: true, AuthorizationCode: "code"}})

	w := postPayment(r, validPaymentJSON(), "")

	require.Equal(t, http.StatusCreated, w.Code)

	var payment models.Payment
	require.NoError(t, json.NewDecoder(w.Body).Decode(&payment))
	assert.Equal(t, models.StatusAuthorized, payment.Status)
	assert.Equal(t, "8877", payment.CardNumberLastFour)
}

func TestPostHandler_Declined(t *testing.T) {
	r, _ := newTestRouter(&stubBankClient{resp: &bank.AuthorizeResponse{Authorized: false}})

	w := postPayment(r, validPaymentJSON(), "")

	require.Equal(t, http.StatusCreated, w.Code)

	var payment models.Payment
	require.NoError(t, json.NewDecoder(w.Body).Decode(&payment))
	assert.Equal(t, models.StatusDeclined, payment.Status)
}

func TestPostHandler_Rejected_InvalidField(t *testing.T) {
	r, _ := newTestRouter(&stubBankClient{resp: &bank.AuthorizeResponse{Authorized: true}})

	invalid, _ := json.Marshal(models.PostPaymentRequest{
		CardNumber:  "123", // too short
		ExpiryMonth: 4,
		ExpiryYear:  time.Now().Year() + 2,
		Currency:    "GBP",
		Amount:      100,
		Cvv:         "123",
	})

	w := postPayment(r, invalid, "")

	require.Equal(t, http.StatusBadRequest, w.Code)

	var errResp errorResponse
	require.NoError(t, json.NewDecoder(w.Body).Decode(&errResp))
	assert.Equal(t, "validation_error", errResp.Error.Code)
	assert.Contains(t, errResp.Error.Fields, "card_number")
}

func TestPostHandler_MalformedJSON(t *testing.T) {
	r, _ := newTestRouter(&stubBankClient{})

	w := postPayment(r, []byte("{not-json"), "")

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestPostHandler_BankUnavailable(t *testing.T) {
	r, _ := newTestRouter(&stubBankClient{err: bank.ErrUnavailable})

	w := postPayment(r, validPaymentJSON(), "")

	assert.Equal(t, http.StatusBadGateway, w.Code)
}

func TestPostHandler_Idempotency_ReplaysCachedResponse(t *testing.T) {
	bankClient := &stubBankClient{resp: &bank.AuthorizeResponse{Authorized: true, AuthorizationCode: "code"}}
	r, _ := newTestRouter(bankClient)
	body := validPaymentJSON()

	first := postPayment(r, body, "same-key")
	second := postPayment(r, body, "same-key")

	require.Equal(t, http.StatusCreated, first.Code)
	assert.Equal(t, first.Code, second.Code)
	assert.Equal(t, first.Body.String(), second.Body.String())
	assert.Equal(t, 1, bankClient.callCount(), "a replayed request must not call the bank again")
}

func TestPostHandler_Idempotency_ConflictOnDifferentBody(t *testing.T) {
	bankClient := &stubBankClient{resp: &bank.AuthorizeResponse{Authorized: true}}
	r, _ := newTestRouter(bankClient)

	first := postPayment(r, validPaymentJSON(), "reused-key")
	require.Equal(t, http.StatusCreated, first.Code)

	differentBody, _ := json.Marshal(models.PostPaymentRequest{
		CardNumber:  "2222405343248877",
		ExpiryMonth: 4,
		ExpiryYear:  time.Now().Year() + 2,
		Currency:    "GBP",
		Amount:      999, // different amount, same key
		Cvv:         "123",
	})
	second := postPayment(r, differentBody, "reused-key")

	assert.Equal(t, http.StatusUnprocessableEntity, second.Code)
}

func TestPostHandler_Idempotency_ReleasedAfterBankUnavailable_AllowsRetry(t *testing.T) {
	bankClient := &sequencedBankClient{results: []struct {
		resp *bank.AuthorizeResponse
		err  error
	}{
		{nil, bank.ErrUnavailable},
		{&bank.AuthorizeResponse{Authorized: true}, nil},
	}}
	r, _ := newTestRouter(bankClient)
	body := validPaymentJSON()

	first := postPayment(r, body, "retry-key")
	require.Equal(t, http.StatusBadGateway, first.Code)

	second := postPayment(r, body, "retry-key")
	require.Equal(t, http.StatusCreated, second.Code)
}

func TestPostHandler_Idempotency_ConcurrentDuplicateGetsConflict(t *testing.T) {
	bankClient := &blockingBankClient{
		entered: make(chan struct{}),
		release: make(chan struct{}),
		resp:    &bank.AuthorizeResponse{Authorized: true, AuthorizationCode: "code"},
	}
	r, _ := newTestRouter(bankClient)
	body := validPaymentJSON()

	var wg sync.WaitGroup
	var firstStatus int
	wg.Add(1)
	go func() {
		defer wg.Done()
		firstStatus = postPayment(r, body, "concurrent-key").Code
	}()

	<-bankClient.entered // first request is now blocked inside the bank call, key is reserved

	second := postPayment(r, body, "concurrent-key")
	assert.Equal(t, http.StatusConflict, second.Code)

	close(bankClient.release)
	wg.Wait()
	assert.Equal(t, http.StatusCreated, firstStatus)
}

func TestGetHandler_PaymentFound(t *testing.T) {
	r, repo := newTestRouter(&stubBankClient{})
	payment := models.Payment{
		ID:                 "test-id",
		Status:             models.StatusAuthorized,
		CardNumberLastFour: "1234",
		ExpiryMonth:        10,
		ExpiryYear:         2035,
		Currency:           "GBP",
		Amount:             100,
	}
	repo.Save(payment)

	req := httptest.NewRequest(http.MethodGet, "/api/payments/test-id", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	var got models.Payment
	require.NoError(t, json.NewDecoder(w.Body).Decode(&got))
	assert.Equal(t, payment, got)
}

func TestGetHandler_PaymentNotFound(t *testing.T) {
	r, _ := newTestRouter(&stubBankClient{})

	req := httptest.NewRequest(http.MethodGet, "/api/payments/NonExistingID", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

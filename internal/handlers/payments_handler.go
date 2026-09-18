// Package handlers is the thin HTTP layer: decode, delegate to the service,
// map the outcome to a status code. No payment logic lives here.
package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/cko-recruitment/payment-gateway-challenge-go/internal/idempotency"
	"github.com/cko-recruitment/payment-gateway-challenge-go/internal/models"
	"github.com/cko-recruitment/payment-gateway-challenge-go/internal/service"
	"github.com/go-chi/chi/v5"
)

type PaymentsHandler struct {
	service     *service.PaymentService
	idempotency *idempotency.Store
}

func NewPaymentsHandler(s *service.PaymentService) *PaymentsHandler {
	return &PaymentsHandler{
		service:     s,
		idempotency: idempotency.NewStore(),
	}
}

// PostHandler godoc
//
//	@Summary		Process a payment
//	@Description	Submits a card payment to the acquiring bank and returns its outcome. An optional Idempotency-Key header makes a retried request replay the original outcome instead of being reprocessed.
//	@Tags			payments
//	@Accept			json
//	@Produce		json
//	@Param			Idempotency-Key	header		string						false	"Client-generated key that makes retrying this request safe"
//	@Param			request			body		models.PostPaymentRequest	true	"Payment details"
//	@Success		201				{object}	models.Payment
//	@Failure		400				{object}	errorResponse	"Rejected: invalid request"
//	@Failure		409				{object}	errorResponse	"A request with this Idempotency-Key is already in progress"
//	@Failure		422				{object}	errorResponse	"Idempotency-Key reused with a different request body"
//	@Failure		502				{object}	errorResponse	"Bank unavailable"
//	@Router			/api/payments [post]
func (h *PaymentsHandler) PostHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid_request", "could not read request body", nil)
			return
		}

		var req models.PostPaymentRequest
		if err := json.Unmarshal(body, &req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_request", "malformed JSON body", nil)
			return
		}

		idemKey := r.Header.Get("Idempotency-Key")
		if idemKey != "" {
			reqHash := idempotency.Hash(body)
			if existing, found := h.idempotency.Reserve(idemKey, reqHash); found {
				h.replayOrConflict(w, existing, reqHash)
				return
			}
		}

		status, respBody := h.createPayment(r.Context(), req)

		if idemKey != "" {
			if isRetryable(status) {
				// Unknown outcome, so free the key for a real retry.
				h.idempotency.Release(idemKey)
			} else {
				h.idempotency.Complete(idemKey, status, respBody)
			}
		}

		writeRaw(w, status, respBody)
	}
}

// GetHandler godoc
//
//	@Summary		Retrieve a payment
//	@Description	Retrieves the details of a previously processed payment by its ID.
//	@Tags			payments
//	@Produce		json
//	@Param			id	path		string	true	"Payment ID"
//	@Success		200	{object}	models.Payment
//	@Failure		404	{object}	errorResponse	"Payment not found"
//	@Router			/api/payments/{id} [get]
func (h *PaymentsHandler) GetHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")

		payment, err := h.service.GetPayment(id)
		if errors.Is(err, service.ErrPaymentNotFound) {
			writeError(w, http.StatusNotFound, "not_found", "no payment exists with the given id", nil)
			return
		}

		writeJSON(w, http.StatusOK, payment)
	}
}

// createPayment maps the service's outcome to a status code and body.
func (h *PaymentsHandler) createPayment(ctx context.Context, req models.PostPaymentRequest) (int, []byte) {
	payment, verr, err := h.service.CreatePayment(ctx, req)
	switch {
	case verr != nil:
		return http.StatusBadRequest, mustMarshal(errorResponse{
			Error: errorDetail{Code: "validation_error", Fields: verr.Fields},
		})
	case errors.Is(err, service.ErrBankUnavailable):
		return http.StatusBadGateway, mustMarshal(errorResponse{
			Error: errorDetail{Code: "bank_unavailable", Message: "the acquiring bank could not process this payment, please retry"},
		})
	case err != nil:
		return http.StatusInternalServerError, mustMarshal(errorResponse{
			Error: errorDetail{Code: "internal_error", Message: "an unexpected error occurred"},
		})
	default:
		return http.StatusCreated, mustMarshal(payment)
	}
}

// replayOrConflict replays a matching cached response, rejects a reused key
// with a different body, or reports the original request as still in flight.
func (h *PaymentsHandler) replayOrConflict(w http.ResponseWriter, rec *idempotency.Record, reqHash string) {
	if rec.State == idempotency.StateCompleted {
		if rec.RequestHash != reqHash {
			writeError(w, http.StatusUnprocessableEntity, "idempotency_key_conflict",
				"this Idempotency-Key was already used with a different request body", nil)
			return
		}
		writeRaw(w, rec.StatusCode, rec.Body)
		return
	}

	writeError(w, http.StatusConflict, "request_in_progress",
		"a request with this Idempotency-Key is already being processed", nil)
}

// isRetryable reports whether a response is an unknown outcome that a retry
// should be allowed to re-attempt, rather than a terminal one to replay.
func isRetryable(status int) bool {
	return status == http.StatusBadGateway || status == http.StatusInternalServerError
}

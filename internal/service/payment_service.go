// Package service holds the payment gateway's business logic: validate,
// call the bank, persist the outcome, record metrics.
package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/cko-recruitment/payment-gateway-challenge-go/internal/bank"
	"github.com/cko-recruitment/payment-gateway-challenge-go/internal/models"
	"github.com/cko-recruitment/payment-gateway-challenge-go/internal/platform"
	"github.com/cko-recruitment/payment-gateway-challenge-go/internal/validation"
)

// ErrPaymentNotFound is returned by GetPayment when no payment exists for the given ID.
var ErrPaymentNotFound = errors.New("payment not found")

// ErrBankUnavailable is returned when the bank couldn't be reached, timed
// out, or errored. Nothing is persisted in this case.
var ErrBankUnavailable = errors.New("bank unavailable")

// Repository is the persistence dependency PaymentService needs, declared
// here so it can be tested with a fake.
type Repository interface {
	Get(id string) (models.Payment, bool)
	Save(payment models.Payment)
}

type PaymentService struct {
	repo Repository
	bank bank.Client
	now  func() time.Time
}

func NewPaymentService(repo Repository, bankClient bank.Client) *PaymentService {
	return &PaymentService{
		repo: repo,
		bank: bankClient,
		now:  time.Now,
	}
}

// CreatePayment validates the request, then calls the bank and persists the
// outcome. A validation error means Rejected; a non-nil error means the
// bank was unreachable. Neither case persists anything.
func (s *PaymentService) CreatePayment(ctx context.Context, req models.PostPaymentRequest) (*models.Payment, *validation.ValidationError, error) {
	if verr := validation.ValidatePaymentRequest(req, s.now()); verr != nil {
		return nil, verr, nil
	}

	start := time.Now()
	authResp, err := s.bank.Authorize(ctx, bank.AuthorizeRequest{
		CardNumber:  req.CardNumber,
		ExpiryMonth: req.ExpiryMonth,
		ExpiryYear:  req.ExpiryYear,
		Currency:    req.Currency,
		Amount:      req.Amount,
		CVV:         req.Cvv,
	})
	platform.ObserveBankCallDuration(time.Since(start))
	if err != nil {
		platform.IncBankCallError()
		return nil, nil, fmt.Errorf("%w: %v", ErrBankUnavailable, err)
	}

	status := models.StatusDeclined
	if authResp.Authorized {
		status = models.StatusAuthorized
	}

	payment := models.Payment{
		ID:                 newPaymentID(),
		Status:             status,
		CardNumberLastFour: req.CardNumber[len(req.CardNumber)-4:],
		ExpiryMonth:        req.ExpiryMonth,
		ExpiryYear:         req.ExpiryYear,
		Currency:           req.Currency,
		Amount:             req.Amount,
	}
	s.repo.Save(payment)
	platform.RecordPaymentOutcome(status)

	return &payment, nil, nil
}

// GetPayment retrieves a previously processed payment by ID.
func (s *PaymentService) GetPayment(id string) (*models.Payment, error) {
	payment, ok := s.repo.Get(id)
	if !ok {
		return nil, ErrPaymentNotFound
	}
	return &payment, nil
}

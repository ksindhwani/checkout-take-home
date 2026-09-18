package service

import (
	"context"
	"testing"
	"time"

	"github.com/cko-recruitment/payment-gateway-challenge-go/internal/bank"
	"github.com/cko-recruitment/payment-gateway-challenge-go/internal/models"
	"github.com/cko-recruitment/payment-gateway-challenge-go/internal/repository"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeBankClient struct {
	resp *bank.AuthorizeResponse
	err  error
}

func (f *fakeBankClient) Authorize(_ context.Context, _ bank.AuthorizeRequest) (*bank.AuthorizeResponse, error) {
	return f.resp, f.err
}

func validPaymentRequest() models.PostPaymentRequest {
	return models.PostPaymentRequest{
		CardNumber:  "2222405343248877",
		ExpiryMonth: 4,
		ExpiryYear:  time.Now().Year() + 2,
		Currency:    "GBP",
		Amount:      100,
		Cvv:         "123",
	}
}

func TestCreatePayment_Authorized(t *testing.T) {
	bankClient := &fakeBankClient{resp: &bank.AuthorizeResponse{Authorized: true, AuthorizationCode: "code-1"}}
	repo := repository.NewPaymentsRepository()
	svc := NewPaymentService(repo, bankClient)

	payment, verr, err := svc.CreatePayment(context.Background(), validPaymentRequest())

	require.NoError(t, err)
	require.Nil(t, verr)
	require.NotNil(t, payment)
	assert.Equal(t, models.StatusAuthorized, payment.Status)
	assert.Equal(t, "8877", payment.CardNumberLastFour)
	assert.NotEmpty(t, payment.ID)

	stored, ok := repo.Get(payment.ID)
	require.True(t, ok)
	assert.Equal(t, *payment, stored)
}

func TestCreatePayment_Declined(t *testing.T) {
	bankClient := &fakeBankClient{resp: &bank.AuthorizeResponse{Authorized: false}}
	repo := repository.NewPaymentsRepository()
	svc := NewPaymentService(repo, bankClient)

	payment, verr, err := svc.CreatePayment(context.Background(), validPaymentRequest())

	require.NoError(t, err)
	require.Nil(t, verr)
	require.NotNil(t, payment)
	assert.Equal(t, models.StatusDeclined, payment.Status)

	_, ok := repo.Get(payment.ID)
	assert.True(t, ok, "declined payments must still be persisted")
}

func TestCreatePayment_Rejected_DoesNotCallBank(t *testing.T) {
	bankClient := &fakeBankClient{resp: &bank.AuthorizeResponse{Authorized: true}}
	repo := repository.NewPaymentsRepository()
	svc := NewPaymentService(repo, bankClient)

	req := validPaymentRequest()
	req.Cvv = "1" // invalid

	payment, verr, err := svc.CreatePayment(context.Background(), req)

	require.NoError(t, err)
	require.Nil(t, payment)
	require.NotNil(t, verr)
	assert.Contains(t, verr.Fields, "cvv")
}

func TestCreatePayment_BankUnavailable_DoesNotPersist(t *testing.T) {
	bankClient := &fakeBankClient{err: bank.ErrUnavailable}
	repo := repository.NewPaymentsRepository()
	svc := NewPaymentService(repo, bankClient)

	payment, verr, err := svc.CreatePayment(context.Background(), validPaymentRequest())

	require.Nil(t, payment)
	require.Nil(t, verr)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrBankUnavailable)
}

func TestGetPayment_Found(t *testing.T) {
	repo := repository.NewPaymentsRepository()
	repo.Save(models.Payment{ID: "abc", Status: models.StatusAuthorized})
	svc := NewPaymentService(repo, &fakeBankClient{})

	payment, err := svc.GetPayment("abc")

	require.NoError(t, err)
	assert.Equal(t, "abc", payment.ID)
}

func TestGetPayment_NotFound(t *testing.T) {
	repo := repository.NewPaymentsRepository()
	svc := NewPaymentService(repo, &fakeBankClient{})

	_, err := svc.GetPayment("missing")

	require.Error(t, err)
	assert.ErrorIs(t, err, ErrPaymentNotFound)
}

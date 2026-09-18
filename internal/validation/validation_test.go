package validation

import (
	"testing"
	"time"

	"github.com/cko-recruitment/payment-gateway-challenge-go/internal/models"
	"github.com/stretchr/testify/assert"
)

var fixedNow = time.Date(2026, 6, 15, 0, 0, 0, 0, time.UTC)

func validRequest() models.PostPaymentRequest {
	return models.PostPaymentRequest{
		CardNumber:  "2222405343248877",
		ExpiryMonth: 4,
		ExpiryYear:  2030,
		Currency:    "GBP",
		Amount:      100,
		Cvv:         "123",
	}
}

func TestValidatePaymentRequest_Valid(t *testing.T) {
	err := ValidatePaymentRequest(validRequest(), fixedNow)
	assert.Nil(t, err)
}

func TestValidatePaymentRequest_AggregatesMultipleErrors(t *testing.T) {
	req := validRequest()
	req.CardNumber = "123"
	req.Cvv = "1"

	err := ValidatePaymentRequest(req, fixedNow)
	if assert.NotNil(t, err) {
		assert.Contains(t, err.Fields, "card_number")
		assert.Contains(t, err.Fields, "cvv")
		assert.Len(t, err.Fields, 2)
	}
}

func TestValidateCardNumber(t *testing.T) {
	tests := []struct {
		name    string
		card    string
		wantErr bool
	}{
		{"valid 16 digits", "2222405343248877", false},
		{"minimum length 14", "12345678901234", false},
		{"maximum length 19", "1234567890123456789", false},
		{"too short 13", "1234567890123", true},
		{"too long 20", "12345678901234567890", true},
		{"empty", "", true},
		{"non-numeric", "1234abcd90123456", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateCardNumber(tt.card)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidateExpiry(t *testing.T) {
	tests := []struct {
		name    string
		month   int
		year    int
		wantErr bool
	}{
		{"future date", 7, 2026, false},
		{"current month is still valid", 6, 2026, false},
		{"past month same year", 5, 2026, true},
		{"past year", 12, 2025, true},
		{"month zero", 0, 2030, true},
		{"month 13", 13, 2030, true},
		{"missing both", 0, 0, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateExpiry(tt.month, tt.year, fixedNow)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidateCurrency(t *testing.T) {
	tests := []struct {
		name     string
		currency string
		wantErr  bool
	}{
		{"GBP", "GBP", false},
		{"USD", "USD", false},
		{"EUR", "EUR", false},
		{"unsupported code", "JPY", true},
		{"too short", "GB", true},
		{"too long", "GBPX", true},
		{"empty", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateCurrency(tt.currency)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidateAmount(t *testing.T) {
	tests := []struct {
		name    string
		amount  int
		wantErr bool
	}{
		{"positive", 100, false},
		{"one", 1, false},
		{"zero", 0, true},
		{"negative", -50, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateAmount(tt.amount)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidateCVV(t *testing.T) {
	tests := []struct {
		name    string
		cvv     string
		wantErr bool
	}{
		{"3 digits", "123", false},
		{"4 digits", "1234", false},
		{"2 digits", "12", true},
		{"5 digits", "12345", true},
		{"non-numeric", "12a", true},
		{"leading zero preserved as valid", "012", false},
		{"empty", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateCVV(tt.cvv)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

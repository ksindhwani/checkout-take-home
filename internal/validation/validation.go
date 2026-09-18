// Package validation implements the payment-request field rules as small,
// pure functions.
package validation

import (
	"strings"
	"time"

	"github.com/cko-recruitment/payment-gateway-challenge-go/internal/models"
)

// AllowedCurrencies is the fixed allowlist of ISO-4217 codes this gateway
// accepts (the spec caps this at 3).
var AllowedCurrencies = map[string]bool{
	"GBP": true,
	"USD": true,
	"EUR": true,
}

// paymentRules maps each request field to the Rule that validates it. The
// map key doubles as the field name reported in a ValidationError.
var paymentRules = map[string]Rule{
	"card_number": cardNumberRule{},
	"expiry":      expiryRule{},
	"currency":    currencyRule{},
	"amount":      amountRule{},
	"cvv":         cvvRule{},
}

// FieldErrors maps a request field name to a human-readable validation message.
type FieldErrors map[string]string

// Error implements the error interface, so *ValidationError can be returned
// like a plain error while still letting callers recover Fields via errors.As.
type ValidationError struct {
	Fields FieldErrors
}

// Rule validates one field of a payment request against the full request
// (some rules, like expiry, need more than one field) and the current time.
type Rule interface {
	Validate(req models.PostPaymentRequest, now time.Time) error
}

type cardNumberRule struct{}

func (cardNumberRule) Validate(req models.PostPaymentRequest, _ time.Time) error {
	return ValidateCardNumber(req.CardNumber)
}

type expiryRule struct{}

func (expiryRule) Validate(req models.PostPaymentRequest, now time.Time) error {
	return ValidateExpiry(req.ExpiryMonth, req.ExpiryYear, now)
}

type currencyRule struct{}

func (currencyRule) Validate(req models.PostPaymentRequest, _ time.Time) error {
	return ValidateCurrency(req.Currency)
}

type amountRule struct{}

func (amountRule) Validate(req models.PostPaymentRequest, _ time.Time) error {
	return ValidateAmount(req.Amount)
}

type cvvRule struct{}

func (cvvRule) Validate(req models.PostPaymentRequest, _ time.Time) error {
	return ValidateCVV(req.Cvv)
}

func (e *ValidationError) Error() string {
	return "payment request failed validation"
}

// ValidatePaymentRequest runs every rule in paymentRules and aggregates all
// failures, not just the first.
func ValidatePaymentRequest(req models.PostPaymentRequest, now time.Time) *ValidationError {
	fields := FieldErrors{}
	for field, rule := range paymentRules {
		if err := rule.Validate(req, now); err != nil {
			fields[field] = err.Error()
		}
	}

	if len(fields) == 0 {
		return nil
	}
	return &ValidationError{Fields: fields}
}

func ValidateCardNumber(cardNumber string) error {
	if cardNumber == "" {
		return errRequired
	}
	if len(cardNumber) < 14 || len(cardNumber) > 19 {
		return errString("must be between 14 and 19 characters long")
	}
	if !isNumeric(cardNumber) {
		return errString("must only contain numeric characters")
	}
	return nil
}

func ValidateExpiry(month, year int, now time.Time) error {
	if month == 0 && year == 0 {
		return errRequired
	}
	if month < 1 || month > 12 {
		return errString("expiry month must be between 1 and 12")
	}
	if year == 0 {
		return errRequired
	}

	// The last valid instant for a card is the end of its expiry month.
	expiry := time.Date(year, time.Month(month)+1, 1, 0, 0, 0, 0, time.UTC)
	if !expiry.After(now) {
		return errString("expiry month/year must be in the future")
	}
	return nil
}

func ValidateCurrency(currency string) error {
	if currency == "" {
		return errRequired
	}
	if len(currency) != 3 {
		return errString("must be 3 characters")
	}
	if !AllowedCurrencies[strings.ToUpper(currency)] {
		return errString("must be one of the supported currencies")
	}
	return nil
}

func ValidateAmount(amount int) error {
	if amount <= 0 {
		return errString("must be a positive integer")
	}
	return nil
}

func ValidateCVV(cvv string) error {
	if cvv == "" {
		return errRequired
	}
	if len(cvv) < 3 || len(cvv) > 4 {
		return errString("must be 3-4 characters long")
	}
	if !isNumeric(cvv) {
		return errString("must only contain numeric characters")
	}
	return nil
}

func isNumeric(s string) bool {
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

type errString string

func (e errString) Error() string { return string(e) }

const errRequired = errString("is required")

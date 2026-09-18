package models

// PaymentStatus represents the terminal state of a payment attempt.
type PaymentStatus string

const (
	StatusAuthorized PaymentStatus = "Authorized"
	StatusDeclined   PaymentStatus = "Declined"
	StatusRejected   PaymentStatus = "Rejected"
)

// PostPaymentRequest is the wire format a merchant submits to process a payment.
type PostPaymentRequest struct {
	CardNumber  string `json:"card_number"`
	ExpiryMonth int    `json:"expiry_month"`
	ExpiryYear  int    `json:"expiry_year"`
	Currency    string `json:"currency"`
	Amount      int    `json:"amount"`
	Cvv         string `json:"cvv"`
}

// Payment is both the persisted record and the API response for a payment.
// One type is used since storage and the wire format never need to differ.
type Payment struct {
	ID                 string        `json:"id"`
	Status             PaymentStatus `json:"status"`
	CardNumberLastFour string        `json:"card_number_last_four"`
	ExpiryMonth        int           `json:"expiry_month"`
	ExpiryYear         int           `json:"expiry_year"`
	Currency           string        `json:"currency"`
	Amount             int           `json:"amount"`
}

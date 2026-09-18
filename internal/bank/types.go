package bank

// AuthorizeRequest is what the gateway asks the acquiring bank to authorize.
type AuthorizeRequest struct {
	CardNumber  string
	ExpiryMonth int
	ExpiryYear  int
	Currency    string
	Amount      int
	CVV         string
}

// AuthorizeResponse is the bank's decision for an authorize request.
type AuthorizeResponse struct {
	Authorized        bool
	AuthorizationCode string
}

// wireRequest is the JSON shape the Mountebank bank simulator expects.
type wireRequest struct {
	CardNumber string `json:"card_number"`
	ExpiryDate string `json:"expiry_date"`
	Currency   string `json:"currency"`
	Amount     int    `json:"amount"`
	Cvv        string `json:"cvv"`
}

// wireResponse is the JSON shape the Mountebank bank simulator returns on 200.
type wireResponse struct {
	Authorized        bool   `json:"authorized"`
	AuthorizationCode string `json:"authorization_code"`
}

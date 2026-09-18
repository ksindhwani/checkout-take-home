package repository

import (
	"sync"

	"github.com/cko-recruitment/payment-gateway-challenge-go/internal/models"
)

// PaymentsRepository is an in-memory, concurrency-safe payment store.
type PaymentsRepository struct {
	mu       sync.RWMutex
	payments map[string]models.Payment
}

func NewPaymentsRepository() *PaymentsRepository {
	return &PaymentsRepository{
		payments: make(map[string]models.Payment),
	}
}

// Get returns the payment with the given ID, and whether it was found.
func (r *PaymentsRepository) Get(id string) (models.Payment, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	payment, ok := r.payments[id]
	return payment, ok
}

// Save inserts or replaces the payment record keyed by its ID.
func (r *PaymentsRepository) Save(payment models.Payment) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.payments[payment.ID] = payment
}

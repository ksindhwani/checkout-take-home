package repository

import (
	"fmt"
	"sync"
	"testing"

	"github.com/cko-recruitment/payment-gateway-challenge-go/internal/models"
	"github.com/stretchr/testify/assert"
)

func TestPaymentsRepository_SaveAndGet(t *testing.T) {
	repo := NewPaymentsRepository()

	payment := models.Payment{
		ID:                 "test-id",
		Status:             models.StatusAuthorized,
		CardNumberLastFour: "0002",
		ExpiryMonth:        10,
		ExpiryYear:         2035,
		Currency:           "GBP",
		Amount:             100,
	}
	repo.Save(payment)

	got, ok := repo.Get("test-id")
	assert.True(t, ok)
	assert.Equal(t, payment, got)
}

func TestPaymentsRepository_GetNotFound(t *testing.T) {
	repo := NewPaymentsRepository()

	_, ok := repo.Get("does-not-exist")
	assert.False(t, ok)
}

// TestPaymentsRepository_ConcurrentAccess exercises the repository with the
// race detector (`go test -race`) to confirm concurrent reads/writes are safe.
func TestPaymentsRepository_ConcurrentAccess(t *testing.T) {
	repo := NewPaymentsRepository()

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(2)
		id := fmt.Sprintf("id-%d", i)

		go func() {
			defer wg.Done()
			repo.Save(models.Payment{ID: id, Status: models.StatusAuthorized})
		}()

		go func() {
			defer wg.Done()
			repo.Get(id)
		}()
	}
	wg.Wait()
}

package idempotency

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestStore_Reserve_FirstCallClaimsKey(t *testing.T) {
	s := NewStore()

	rec, found := s.Reserve("key-1", "hash-a")

	assert.False(t, found)
	assert.Nil(t, rec)
}

func TestStore_Reserve_SecondCallSeesInProgress(t *testing.T) {
	s := NewStore()
	s.Reserve("key-1", "hash-a")

	rec, found := s.Reserve("key-1", "hash-a")

	if assert.True(t, found) {
		assert.Equal(t, StateInProgress, rec.State)
	}
}

func TestStore_CompleteThenReserve_ReplaysRecord(t *testing.T) {
	s := NewStore()
	s.Reserve("key-1", "hash-a")
	s.Complete("key-1", 201, []byte(`{"id":"p1"}`))

	rec, found := s.Reserve("key-1", "hash-a")

	if assert.True(t, found) {
		assert.Equal(t, StateCompleted, rec.State)
		assert.Equal(t, 201, rec.StatusCode)
		assert.Equal(t, `{"id":"p1"}`, string(rec.Body))
		assert.Equal(t, "hash-a", rec.RequestHash)
	}
}

func TestStore_Release_FreesKeyForRetry(t *testing.T) {
	s := NewStore()
	s.Reserve("key-1", "hash-a")
	s.Release("key-1")

	rec, found := s.Reserve("key-1", "hash-a")

	assert.False(t, found)
	assert.Nil(t, rec)
}

func TestStore_ConcurrentReserve_OnlyOneCallerClaimsKey(t *testing.T) {
	s := NewStore()

	const n = 50
	var wg sync.WaitGroup
	claims := make(chan bool, n)

	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, found := s.Reserve("shared-key", "hash-a")
			claims <- !found
		}()
	}
	wg.Wait()
	close(claims)

	claimed := 0
	for c := range claims {
		if c {
			claimed++
		}
	}
	assert.Equal(t, 1, claimed, "exactly one concurrent caller should claim the key")
}

func TestHash_SameBodySameHash(t *testing.T) {
	assert.Equal(t, Hash([]byte("abc")), Hash([]byte("abc")))
}

func TestHash_DifferentBodyDifferentHash(t *testing.T) {
	assert.NotEqual(t, Hash([]byte("abc")), Hash([]byte("xyz")))
}

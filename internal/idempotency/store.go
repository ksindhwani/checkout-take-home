// Package idempotency replays a retried request's original outcome instead
// of reprocessing it, so a retry doesn't call the bank twice.
package idempotency

import (
	"crypto/sha256"
	"encoding/hex"
	"sync"
)

type State string

const (
	StateInProgress State = "in_progress"
	StateCompleted  State = "completed"
)

// Record is what's stored against a single Idempotency-Key.
type Record struct {
	// RequestHash fingerprints the request body, to detect key reuse with a
	// different payload.
	RequestHash string
	State       State
	StatusCode  int
	Body        []byte
}

// Store tracks in-flight and completed requests keyed by Idempotency-Key.
// Plain in-memory map, no TTL or eviction.
type Store struct {
	mu      sync.Mutex
	records map[string]*Record
}

func NewStore() *Store {
	return &Store{records: make(map[string]*Record)}
}

func (s *Store) Reserve(key, requestHash string) (record *Record, found bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if rec, ok := s.records[key]; ok {
		return rec, true
	}
	s.records[key] = &Record{RequestHash: requestHash, State: StateInProgress}
	return nil, false
}

// Complete stores the terminal, replayable outcome for a previously reserved key.
func (s *Store) Complete(key string, statusCode int, body []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if rec, ok := s.records[key]; ok {
		rec.State = StateCompleted
		rec.StatusCode = statusCode
		rec.Body = body
	}
}

// Release frees a reserved key without an outcome, so a retry can attempt
// the bank call again instead of replaying a failure forever.
func (s *Store) Release(key string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.records, key)
}

// Hash returns a stable fingerprint of a request body.
func Hash(body []byte) string {
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:])
}

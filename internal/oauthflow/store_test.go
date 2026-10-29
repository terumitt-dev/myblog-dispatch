package oauthflow

import (
	"errors"
	"testing"
	"time"
)

func TestStore_NewAndConsume(t *testing.T) {
	s := NewStore(5 * time.Minute)

	state, verifier, err := s.New()
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	if state == "" || verifier == "" {
		t.Fatalf("expected non-empty state and verifier, got state=%q verifier=%q", state, verifier)
	}
	if state == verifier {
		t.Fatal("state and verifier must not be identical")
	}

	got, ok := s.Consume(state)
	if !ok {
		t.Fatal("expected Consume to succeed")
	}
	if got != verifier {
		t.Fatalf("expected verifier %q, got %q", verifier, got)
	}
}

func TestStore_Consume_SingleUse(t *testing.T) {
	s := NewStore(5 * time.Minute)
	state, _, err := s.New()
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}

	if _, ok := s.Consume(state); !ok {
		t.Fatal("expected first Consume to succeed")
	}
	if _, ok := s.Consume(state); ok {
		t.Fatal("expected second Consume of the same state to fail (replay)")
	}
}

func TestStore_Consume_UnknownState(t *testing.T) {
	s := NewStore(5 * time.Minute)
	if _, ok := s.Consume("never-issued"); ok {
		t.Fatal("expected Consume of unknown state to fail")
	}
}

func TestStore_Consume_Expired(t *testing.T) {
	s := NewStore(10 * time.Millisecond)
	state, _, err := s.New()
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}

	time.Sleep(20 * time.Millisecond)

	if _, ok := s.Consume(state); ok {
		t.Fatal("expected Consume of expired state to fail")
	}
}

func TestStore_New_RejectsWhenFull(t *testing.T) {
	s := NewStore(5 * time.Minute)
	for i := 0; i < maxPending; i++ {
		if _, _, err := s.New(); err != nil {
			t.Fatalf("New #%d returned error: %v", i, err)
		}
	}
	if _, _, err := s.New(); !errors.Is(err, ErrTooManyPending) {
		t.Fatalf("expected ErrTooManyPending, got %v", err)
	}
}

func TestStore_New_UniqueValues(t *testing.T) {
	s := NewStore(5 * time.Minute)
	seen := make(map[string]bool)
	for i := 0; i < 50; i++ {
		state, verifier, err := s.New()
		if err != nil {
			t.Fatalf("New returned error: %v", err)
		}
		if seen[state] {
			t.Fatalf("duplicate state generated: %s", state)
		}
		seen[state] = true
		if seen[verifier] {
			t.Fatalf("duplicate verifier generated: %s", verifier)
		}
		seen[verifier] = true
	}
}

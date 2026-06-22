package auth

import (
	"context"
	"testing"
)

func TestStaticTokener_Token(t *testing.T) {
	s := NewStaticTokener("whk_abc123")
	token, err := s.Token(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if token != "whk_abc123" {
		t.Errorf("expected %q, got %q", "whk_abc123", token)
	}
}

func TestStaticTokener_Token_Empty(t *testing.T) {
	s := NewStaticTokener("")
	token, err := s.Token(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if token != "" {
		t.Errorf("expected empty token, got %q", token)
	}
}

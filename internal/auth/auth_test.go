package auth

import "testing"

func TestHashAndVerifyPassword(t *testing.T) {
	hash, err := HashPassword("secret123")
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	if err := VerifyPassword(hash, "secret123"); err != nil {
		t.Errorf("VerifyPassword correct password: %v", err)
	}
	if err := VerifyPassword(hash, "wrong"); err == nil {
		t.Error("VerifyPassword wrong password: expected error, got nil")
	}
}

func TestGenerateToken(t *testing.T) {
	tok1, err := GenerateToken()
	if err != nil {
		t.Fatalf("GenerateToken: %v", err)
	}
	if len(tok1) != 64 {
		t.Errorf("expected 64 hex chars, got %d", len(tok1))
	}
	tok2, _ := GenerateToken()
	if tok1 == tok2 {
		t.Error("expected unique tokens, got identical values")
	}
}

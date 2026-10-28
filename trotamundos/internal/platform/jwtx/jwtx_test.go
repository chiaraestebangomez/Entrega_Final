package jwtx

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func TestIssueAndParse(t *testing.T) {
	priv, pub, err := Generate()
	if err != nil {
		t.Fatal(err)
	}
	signer := NewSigner(priv, "trotamundos-test", time.Minute)
	now := time.Now().UTC()
	token, claims, err := signer.Issue("user-1", "user@example.com", "traveler", now)
	if err != nil {
		t.Fatal(err)
	}
	if claims.Subject != "user-1" || claims.Email != "user@example.com" || claims.Role != "traveler" {
		t.Fatalf("claims inesperados: %+v", claims)
	}
	verifier := NewVerifier(pub, "trotamundos-test")
	got, err := verifier.Parse(token, now)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if got.Subject != "user-1" || got.JTI == "" {
		t.Fatalf("claims parseados: %+v", got)
	}
}

func TestParseRejectsExpired(t *testing.T) {
	priv, pub, _ := Generate()
	signer := NewSigner(priv, "iss", time.Minute)
	now := time.Now().UTC()
	token, _, _ := signer.Issue("u", "e@x.com", "traveler", now.Add(-2*time.Hour))
	verifier := NewVerifier(pub, "iss")
	if _, err := verifier.Parse(token, now); !errors.Is(err, ErrExpiredToken) {
		t.Fatalf("esperaba ErrExpiredToken, obtuve %v", err)
	}
}

func TestParseRejectsTamperedToken(t *testing.T) {
	priv, pub, _ := Generate()
	signer := NewSigner(priv, "iss", time.Minute)
	now := time.Now().UTC()
	token, _, _ := signer.Issue("u", "e@x.com", "traveler", now)
	parts := strings.Split(token, ".")
	parts[1] = strings.Repeat("A", len(parts[1]))
	verifier := NewVerifier(pub, "iss")
	if _, err := verifier.Parse(strings.Join(parts, "."), now); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("esperaba ErrInvalidToken, obtuve %v", err)
	}
}

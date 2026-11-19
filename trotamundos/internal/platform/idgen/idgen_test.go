package idgen

import (
	"errors"
	"testing"
)

func TestUUIDAndTokenAreUnique(t *testing.T) {
	seen := make(map[string]bool, 2000)
	for i := 0; i < 1000; i++ {
		u := UUID()
		if len(u) != 36 || seen[u] {
			t.Fatalf("UUID inválido o repetido: %q", u)
		}
		seen[u] = true
		tok := Token(16)
		if len(tok) != 32 || seen[tok] {
			t.Fatalf("Token inválido o repetido: %q", tok)
		}
		seen[tok] = true
	}
}

func TestRandomFailureFallsBackWithoutPanic(t *testing.T) {
	original := readRandom
	readRandom = func([]byte) (int, error) { return 0, errors.New("rand unavailable") }
	defer func() { readRandom = original }()

	seen := make(map[string]bool, 200)
	for i := 0; i < 100; i++ {
		u := UUID()
		if len(u) != 36 || u[14] != '4' {
			t.Fatalf("UUID de fallback inválido: %q", u)
		}
		if seen[u] {
			t.Fatalf("UUID de fallback repetido: %q", u)
		}
		seen[u] = true

		tok := Token(64)
		if len(tok) != 128 || seen[tok] {
			t.Fatalf("Token de fallback inválido o repetido: %q", tok)
		}
		seen[tok] = true
	}
}

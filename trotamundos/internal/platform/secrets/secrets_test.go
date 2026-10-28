package secrets

import (
	"bytes"
	"strings"
	"testing"
)

func TestEncryptDecryptRoundTrip(t *testing.T) {
	box, err := New(bytes.Repeat([]byte{7}, 32))
	if err != nil {
		t.Fatal(err)
	}
	plain := "refresh-token-secreto"
	enc, err := box.Encrypt(plain)
	if err != nil {
		t.Fatal(err)
	}
	if enc == plain || strings.Contains(enc, plain) {
		t.Fatal("el texto cifrado no debe exponer el texto plano")
	}
	dec, err := box.Decrypt(enc)
	if err != nil {
		t.Fatal(err)
	}
	if dec != plain {
		t.Fatalf("roundtrip = %q", dec)
	}
}

func TestDecryptWithDifferentKeyFails(t *testing.T) {
	box1, _ := New(bytes.Repeat([]byte{1}, 32))
	box2, _ := New(bytes.Repeat([]byte{2}, 32))
	enc, _ := box1.Encrypt("secreto")
	if _, err := box2.Decrypt(enc); err == nil {
		t.Fatal("esperaba error al descifrar con otra clave")
	}
}

func TestNewRejectsInvalidKeyLength(t *testing.T) {
	if _, err := New([]byte("corta")); err == nil {
		t.Fatal("esperaba error con clave inválida")
	}
}

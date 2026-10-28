package password

import (
	"errors"
	"strings"

	"golang.org/x/crypto/bcrypt"
)

var ErrInvalid = errors.New("invalid credentials")

type Hasher struct {
	Cost int
}

func New(cost int) Hasher {
	if cost <= 0 {
		cost = bcrypt.DefaultCost
	}
	return Hasher{Cost: cost}
}

func (h Hasher) Hash(raw string) (string, error) {
	if strings.TrimSpace(raw) == "" {
		return "", errors.New("password is required")
	}
	b, err := bcrypt.GenerateFromPassword([]byte(raw), h.Cost)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func (h Hasher) Verify(hash, raw string) error {
	if bcrypt.CompareHashAndPassword([]byte(hash), []byte(raw)) != nil {
		return ErrInvalid
	}
	return nil
}

package jwtx

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

var (
	ErrInvalidToken = errors.New("invalid token")
	ErrExpiredToken = errors.New("token expired")
)

type Claims struct {
	Subject   string `json:"sub"`
	Email     string `json:"email,omitempty"`
	Role      string `json:"role,omitempty"`
	Issuer    string `json:"iss"`
	IssuedAt  int64  `json:"iat"`
	ExpiresAt int64  `json:"exp"`
	JTI       string `json:"jti"`
}

type Signer struct {
	private *rsa.PrivateKey
	public  *rsa.PublicKey
	issuer  string
	ttl     time.Duration
	kid     string
}

func NewSigner(private *rsa.PrivateKey, issuer string, ttl time.Duration) *Signer {
	kid := ""
	if private != nil {
		kid = keyID(&private.PublicKey)
	}
	return &Signer{private: private, public: &private.PublicKey, issuer: issuer, ttl: ttl, kid: kid}
}

func NewVerifier(public *rsa.PublicKey, issuer string) *Signer {
	return &Signer{public: public, issuer: issuer}
}

func (s *Signer) Issue(userID, email, role string, now time.Time) (string, Claims, error) {
	if s.private == nil {
		return "", Claims{}, errors.New("jwtx: signer has no private key")
	}
	jti, err := randomID()
	if err != nil {
		return "", Claims{}, err
	}
	claims := Claims{
		Subject:   userID,
		Email:     email,
		Role:      role,
		Issuer:    s.issuer,
		IssuedAt:  now.Unix(),
		ExpiresAt: now.Add(s.ttl).Unix(),
		JTI:       jti,
	}
	header := map[string]string{"alg": "RS256", "typ": "JWT"}
	if s.kid != "" {
		header["kid"] = s.kid
	}
	hb, _ := json.Marshal(header)
	cb, _ := json.Marshal(claims)
	signingInput := enc(hb) + "." + enc(cb)
	sum := sha256.Sum256([]byte(signingInput))
	sig, err := rsa.SignPKCS1v15(rand.Reader, s.private, crypto.SHA256, sum[:])
	if err != nil {
		return "", Claims{}, err
	}
	return signingInput + "." + enc(sig), claims, nil
}

func (s *Signer) Parse(token string, now time.Time) (Claims, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return Claims{}, ErrInvalidToken
	}
	hb, err := dec(parts[0])
	if err != nil {
		return Claims{}, ErrInvalidToken
	}
	var header struct {
		Alg string `json:"alg"`
		Typ string `json:"typ"`
	}
	if err := json.Unmarshal(hb, &header); err != nil || header.Alg != "RS256" {
		return Claims{}, ErrInvalidToken
	}
	cb, err := dec(parts[1])
	if err != nil {
		return Claims{}, ErrInvalidToken
	}
	sig, err := dec(parts[2])
	if err != nil {
		return Claims{}, ErrInvalidToken
	}
	sum := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	if err := rsa.VerifyPKCS1v15(s.public, crypto.SHA256, sum[:], sig); err != nil {
		return Claims{}, ErrInvalidToken
	}
	var claims Claims
	if err := json.Unmarshal(cb, &claims); err != nil {
		return Claims{}, ErrInvalidToken
	}
	if claims.Issuer != s.issuer || claims.Subject == "" {
		return Claims{}, ErrInvalidToken
	}
	if now.Unix() >= claims.ExpiresAt {
		return Claims{}, ErrExpiredToken
	}
	if claims.IssuedAt > now.Add(30*time.Second).Unix() {
		return Claims{}, ErrInvalidToken
	}
	return claims, nil
}

func (s *Signer) TTL() time.Duration { return s.ttl }

func LoadOrGenerate(privatePath, publicPath string) (*rsa.PrivateKey, *rsa.PublicKey, error) {
	if privatePath != "" && publicPath != "" {
		if priv, err := loadPrivate(privatePath); err == nil {
			pub, err := loadPublic(publicPath)
			if err != nil {
				return nil, nil, err
			}
			return priv, pub, nil
		} else if !errors.Is(err, os.ErrNotExist) {
			return nil, nil, err
		}
	}
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, nil, err
	}
	if privatePath != "" && publicPath != "" {
		if err := save(privatePath, publicPath, priv); err != nil {
			return nil, nil, err
		}
	}
	return priv, &priv.PublicKey, nil
}

func Generate() (*rsa.PrivateKey, *rsa.PublicKey, error) {
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, nil, err
	}
	return priv, &priv.PublicKey, nil
}

func MarshalPublic(public *rsa.PublicKey) ([]byte, error) {
	der, err := x509.MarshalPKIXPublicKey(public)
	if err != nil {
		return nil, err
	}
	return pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: der}), nil
}

func loadPrivate(path string) (*rsa.PrivateKey, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	block, _ := pem.Decode(b)
	if block == nil {
		return nil, fmt.Errorf("jwtx: invalid private key pem")
	}
	if k, err := x509.ParsePKCS8PrivateKey(block.Bytes); err == nil {
		if rk, ok := k.(*rsa.PrivateKey); ok {
			return rk, nil
		}
	}
	if k, err := x509.ParsePKCS1PrivateKey(block.Bytes); err == nil {
		return k, nil
	}
	return nil, fmt.Errorf("jwtx: unsupported private key")
}

func loadPublic(path string) (*rsa.PublicKey, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	block, _ := pem.Decode(b)
	if block == nil {
		return nil, fmt.Errorf("jwtx: invalid public key pem")
	}
	k, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		if rk, err2 := x509.ParsePKCS1PublicKey(block.Bytes); err2 == nil {
			return rk, nil
		}
		return nil, err
	}
	rk, ok := k.(*rsa.PublicKey)
	if !ok {
		return nil, fmt.Errorf("jwtx: not an RSA public key")
	}
	return rk, nil
}

func save(privatePath, publicPath string, priv *rsa.PrivateKey) error {
	if err := os.MkdirAll(filepath.Dir(privatePath), 0o700); err != nil && filepath.Dir(privatePath) != "." {
		return err
	}
	privDER, err := x509.MarshalPKCS8PrivateKey(priv)
	if err != nil {
		return err
	}
	if err := os.WriteFile(privatePath, pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: privDER}), 0o600); err != nil {
		return err
	}
	pubDER, err := x509.MarshalPKIXPublicKey(&priv.PublicKey)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(publicPath), 0o755); err != nil && filepath.Dir(publicPath) != "." {
		return err
	}
	return os.WriteFile(publicPath, pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: pubDER}), 0o644)
}

func keyID(pub *rsa.PublicKey) string {
	der, err := x509.MarshalPKIXPublicKey(pub)
	if err != nil {
		return "default"
	}
	sum := sha256.Sum256(der)
	return base64.RawURLEncoding.EncodeToString(sum[:6])
}

func enc(b []byte) string { return base64.RawURLEncoding.EncodeToString(b) }

func dec(s string) ([]byte, error) { return base64.RawURLEncoding.DecodeString(s) }

func randomID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

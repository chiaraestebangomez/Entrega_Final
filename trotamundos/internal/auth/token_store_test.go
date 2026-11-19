package auth

import (
	"context"
	"testing"
	"time"
)

func TestMemoryStoreJTIRevocationExpires(t *testing.T) {
	s := NewMemoryRefreshStore()
	base := time.Now()
	s.now = func() time.Time { return base }
	ctx := context.Background()

	if err := s.RevokeJTI(ctx, "jti-1", time.Minute); err != nil {
		t.Fatal(err)
	}
	revoked, err := s.IsRevokedJTI(ctx, "jti-1")
	if err != nil || !revoked {
		t.Fatalf("esperaba JTI revocado, obtuve %v, %v", revoked, err)
	}
	if revoked, _ := s.IsRevokedJTI(ctx, "otro"); revoked {
		t.Fatal("un JTI distinto no debe estar revocado")
	}

	s.now = func() time.Time { return base.Add(2 * time.Minute) }
	revoked, err = s.IsRevokedJTI(ctx, "jti-1")
	if err != nil || revoked {
		t.Fatalf("el JTI debe expirar, obtuve %v, %v", revoked, err)
	}
}

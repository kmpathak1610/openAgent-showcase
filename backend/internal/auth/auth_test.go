package auth

import (
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestHashAndCheck(t *testing.T) {
	s := New("test-secret-32-chars-long-enough-123")
	h, err := s.HashPassword("s3cr3t")
	if err != nil {
		t.Fatal(err)
	}
	if !s.CheckPassword(h, "s3cr3t") {
		t.Fatal("should match")
	}
	if s.CheckPassword(h, "wrong") {
		t.Fatal("should not match")
	}
}

func TestJWTCreateVerify(t *testing.T) {
	s := New("test-secret-32-chars-long-enough-12345")
	uid := uuid.New()
	oid := uuid.New()
	tok, err := s.CreateToken(uid, oid, "member", "a@b.com", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	claims, err := s.VerifyToken(tok)
	if err != nil {
		t.Fatal(err)
	}
	if claims.UserID != uid || claims.OrganizationID != oid {
		t.Fatal("claims mismatch")
	}
	if _, err := s.VerifyToken(tok + "x"); err == nil {
		t.Fatal("should fail on tampered token")
	}
}

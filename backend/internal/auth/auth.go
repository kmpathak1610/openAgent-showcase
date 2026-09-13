package auth

import (
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

type Claims struct {
	UserID         uuid.UUID `json:"uid"`
	OrganizationID uuid.UUID `json:"org"`
	Role           string    `json:"role"`
	Email          string    `json:"email"`
	jwt.RegisteredClaims
}

type Service struct {
	secret []byte
}

func New(secret string) *Service {
	return &Service{secret: []byte(secret)}
}

func (s *Service) HashPassword(password string) (string, error) {
	b, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	return string(b), err
}

func (s *Service) CheckPassword(hash, password string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
}

func (s *Service) CreateToken(userID, orgID uuid.UUID, role, email string, ttl time.Duration) (string, error) {
	claims := Claims{
		UserID:         userID,
		OrganizationID: orgID,
		Role:           role,
		Email:          email,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   userID.String(),
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(ttl)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return tok.SignedString(s.secret)
}

func (s *Service) VerifyToken(tokenStr string) (*Claims, error) {
	tok, err := jwt.ParseWithClaims(tokenStr, &Claims{}, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errors.New("unexpected signing method")
		}
		return s.secret, nil
	})
	if err != nil {
		return nil, err
	}
	claims, ok := tok.Claims.(*Claims)
	if !ok || !tok.Valid {
		return nil, errors.New("invalid token")
	}
	// Ensure sub parses as UUID and sync UserID if needed
	if claims.Subject != "" {
		if uid, err := uuid.Parse(claims.Subject); err == nil {
			claims.UserID = uid
		} else {
			return nil, errors.New("invalid subject")
		}
	} else if claims.UserID != uuid.Nil {
		claims.Subject = claims.UserID.String()
	}
	if claims.UserID == uuid.Nil {
		return nil, errors.New("invalid subject")
	}
	return claims, nil
}

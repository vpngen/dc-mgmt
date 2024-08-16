package jwtauth

import (
	"crypto"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

type Issuer struct {
	key     crypto.PrivateKey
	options Options
}

func NewIssuer(key crypto.PrivateKey, options Options) *Issuer {
	return &Issuer{key: key, options: options}
}

func (i Issuer) CreateToken(ttl time.Duration, scopes ...string) *Claims {
	now := time.Now().UTC()

	return &Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    i.options.Issuer,
			Subject:   i.options.Subject,
			Audience:  i.options.Audience,
			ExpiresAt: jwt.NewNumericDate(now.Add(ttl)),
			NotBefore: jwt.NewNumericDate(now),
			IssuedAt:  jwt.NewNumericDate(now),
			ID:        uuid.New().String(),
		},
		ClientID: i.options.ClientID,
		Scopes:   scopes,
	}
}

func (i Issuer) Sign(claims *Claims) (string, error) {
	sign, err := jwt.NewWithClaims(i.options.SigningMethod, claims).SignedString(i.key)
	if err != nil {
		return "", fmt.Errorf("sign token: %w", err)
	}

	return sign, nil
}

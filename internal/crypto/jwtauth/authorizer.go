package jwtauth

import (
	"crypto"
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

var (
	ErrTokenUnexpectedSigningMethod = errors.New("unexpected signing method")
	ErrTokenInvalid                 = errors.New("invalid token")
	ErrAudienceUnknown              = errors.New("unknown audience")
	ErrSubjectUnknown               = errors.New("unknown subject")
	ErrMissingScopes                = errors.New("missing scopes")
	ErrTokenExpired                 = errors.New("token expired")
)

type Authorizer struct {
	key     crypto.PublicKey
	options Options
}

func NewAuthorizer(key crypto.PublicKey, options Options) Authorizer {
	return Authorizer{key: key, options: options}
}

func (a Authorizer) Validate(tokenStr string) (*Claims, error) {
	claims := &Claims{}

	token, err := jwt.ParseWithClaims(
		tokenStr,
		claims,
		func(token *jwt.Token) (interface{}, error) {
			if token.Method.Alg() != a.options.SigningMethod.Alg() {
				return nil, ErrTokenUnexpectedSigningMethod
			}
			return a.key, nil
		},
	)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrTokenInvalid, err)
	}

	if !token.Valid {
		return nil, ErrTokenInvalid
	}

	tokenExpirationTime, err := claims.GetExpirationTime()
	if err != nil {
		return nil, ErrTokenInvalid
	}

	if tokenExpirationTime.Time.Before(time.Now().UTC()) {
		return nil, ErrTokenExpired
	}

	return claims, nil
}

func (a Authorizer) Authorize(claims *Claims, scopes ...string) error {
	if a.options.Issuer != "" && claims.Issuer != a.options.Issuer {
		return ErrMissingScopes
	}
	if a.options.Subject != "" && claims.Subject != a.options.Subject {
		return ErrSubjectUnknown
	}
	for _, aud := range a.options.Audience {
		if !sliceContains(claims.Audience, aud) {
			return ErrAudienceUnknown
		}
	}
	for _, scope := range scopes {
		if !sliceContains(claims.Scopes, scope) {
			return ErrMissingScopes
		}
	}
	return nil
}

func sliceContains[T comparable](slice []T, val T) bool {
	for _, v := range slice {
		if v == val {
			return true
		}
	}
	return false
}

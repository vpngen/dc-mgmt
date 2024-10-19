package auth

import (
	"github.com/golang-jwt/jwt/v5"
	"github.com/vpngen/dc-mgmt/internal/crypto/jwtauth"
)

const (
	ThisIssuer   = "vgss"
	ThisAudience = "https://api.vpngen.com"
)

func NewOptions(m jwt.SigningMethod) jwtauth.Options {
	return jwtauth.Options{
		Issuer:        ThisIssuer,
		Audience:      []string{ThisAudience},
		SigningMethod: m,
	}
}

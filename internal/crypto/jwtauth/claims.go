package jwtauth

import "github.com/golang-jwt/jwt/v5"

type Claims struct {
	jwt.RegisteredClaims
	ClientID string   `json:"client_id"`
	Scopes   []string `json:"scopes"`
}

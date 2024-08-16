package jwtauth

import "github.com/golang-jwt/jwt/v5"

type Options struct {
	// if Issuer is empty, all issuers are allowed
	Issuer string
	// if Subject is empty, all subjects are allowed
	Subject string
	// if Audience is empty, all audiences are allowed
	Audience []string

	SigningMethod jwt.SigningMethod

	ClientID string
}

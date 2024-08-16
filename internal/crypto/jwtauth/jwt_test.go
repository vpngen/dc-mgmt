package jwtauth

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/rsa"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/ssh"
)

const (
	ed25519PrivKeyPem = `-----BEGIN OPENSSH PRIVATE KEY-----
b3BlbnNzaC1rZXktdjEAAAAABG5vbmUAAAAEbm9uZQAAAAAAAAABAAAAMwAAAAtzc2gtZW
QyNTUxOQAAACAIGmIU12kslVRVM8GHwJnBi5W5h8tuX93PYaPfH948EAAAAJD15Ckn9eQp
JwAAAAtzc2gtZWQyNTUxOQAAACAIGmIU12kslVRVM8GHwJnBi5W5h8tuX93PYaPfH948EA
AAAEA7OhY0gJqaJp5Kx0EtkyuPiGiZmlWTn/v2X1EN2W7HlAgaYhTXaSyVVFUzwYfAmcGL
lbmHy25f3c9ho98f3jwQAAAAC3BoaWxAd3R2bGFiAQI=
-----END OPENSSH PRIVATE KEY-----`

	ecdsaPrivKeyPem = `-----BEGIN OPENSSH PRIVATE KEY-----
b3BlbnNzaC1rZXktdjEAAAAABG5vbmUAAAAEbm9uZQAAAAAAAAABAAAAaAAAABNlY2RzYS
1zaGEyLW5pc3RwMjU2AAAACG5pc3RwMjU2AAAAQQRd/ZusZAITCDzc6Z97w5Cmg7BLpqKL
kYbSECOSxJgnlLGKjVtRWfGXKa5KqNo980hIYGqOclSJila1bNLMX+1YAAAAqNs8q4nbPK
uJAAAAE2VjZHNhLXNoYTItbmlzdHAyNTYAAAAIbmlzdHAyNTYAAABBBF39m6xkAhMIPNzp
n3vDkKaDsEumoouRhtIQI5LEmCeUsYqNW1FZ8Zcprkqo2j3zSEhgao5yVImKVrVs0sxf7V
gAAAAgeaDELVh5I6w0HEe1jiIg0/bDkkzT7yF7Koh6ZoRaMQcAAAALcGhpbEB3dHZsYWIB
AgMEBQ==
-----END OPENSSH PRIVATE KEY-----`

	rsaPrivKeyPem = `-----BEGIN OPENSSH PRIVATE KEY-----
b3BlbnNzaC1rZXktdjEAAAAABG5vbmUAAAAEbm9uZQAAAAAAAAABAAAAlwAAAAdzc2gtcn
NhAAAAAwEAAQAAAIEAxcFQytDI08LLVdi2dVeMcWxGl8eWuYh/CaYKemLH+h4y5SGjiYDL
3P+Ksv1rfkHVoO0PUXTtoM9AJOaut10wI9IMRr7uuXTWsqT0PnWaOgbO1xpKBT1hcQH51L
vYjD2iOVWgJvcyn+jbnm7lavpmcfFtRmi/SnFcYwbOA6r4fe0AAAIIAA8dlwAPHZcAAAAH
c3NoLXJzYQAAAIEAxcFQytDI08LLVdi2dVeMcWxGl8eWuYh/CaYKemLH+h4y5SGjiYDL3P
+Ksv1rfkHVoO0PUXTtoM9AJOaut10wI9IMRr7uuXTWsqT0PnWaOgbO1xpKBT1hcQH51LvY
jD2iOVWgJvcyn+jbnm7lavpmcfFtRmi/SnFcYwbOA6r4fe0AAAADAQABAAAAgQCL8+UmtB
385/YZejae0ufc+aD4F9OO2I/3lyABP1mBpM+mI2lmjdU5QUy6oejqQNNcgYj+v/7QePxP
YUazFGtVH4bxjnTRMjDOUY1Ak98tet32ql98TqLxEYCdFbSEn5RyLPyrFR/XyJnNuVJBTQ
58T1PvlkoY6jeNedJre74IbQAAAEA50wbM0ZsGlzsHIRbev9vX81Buhk3WY+B3eceI+K6w
7FfCoi3qXUfWg7vlZWjByvKKF5IpBRyqQOgZstnee68ZAAAAQQDtm3JJxJtmrjs3W0/Ltd
2gplVzXF4VQ3hTrv7LVFtc6VNGlCBuEpI9lykYiXZFvpCJadkI44zjXWMO4gRGEunDAAAA
QQDVECOJ9FTvh8TZIt6S2Azt1KVOYjcukWhrsh5vuJMf2THSlOoPyljabDxQju45XMIP5K
gP41b02XPQsJhWr86PAAAAC3BoaWxAd3R2bGFiAQIDBAUGBw==
-----END OPENSSH PRIVATE KEY-----`

	secret = "PinnEspAumNiUrlinuctyirdarajArr4"
)

func TestJWT(t *testing.T) {
	type testCase struct {
		name   string
		key    crypto.PrivateKey
		method jwt.SigningMethod
	}

	ed25519PrivKey, err := ssh.ParseRawPrivateKey([]byte(ed25519PrivKeyPem))
	if err != nil {
		t.Fatal(err)
	}

	ec256PrivKey, err := ssh.ParseRawPrivateKey([]byte(ecdsaPrivKeyPem))
	if err != nil {
		t.Fatal(err)
	}

	rsaPrivKey, err := ssh.ParseRawPrivateKey([]byte(rsaPrivKeyPem))
	if err != nil {
		t.Fatal(err)
	}

	testCases := []testCase{
		{"EdDSA", ed25519PrivKey, jwt.SigningMethodEdDSA},
		{"ES256", ec256PrivKey, jwt.SigningMethodES256},
		{"PS256", rsaPrivKey, jwt.SigningMethodPS256},
		{"HS256", []byte(secret), jwt.SigningMethodHS256},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var key interface{}

			switch tc.method {
			case jwt.SigningMethodEdDSA:
				if k, ok := tc.key.(*ed25519.PrivateKey); ok {
					if pk, ok := k.Public().(ed25519.PublicKey); ok {
						key = pk
					}
				}
			case jwt.SigningMethodES256, jwt.SigningMethodES384, jwt.SigningMethodES512:
				if k, ok := tc.key.(*ecdsa.PrivateKey); ok {
					key = &k.PublicKey
				}
			case jwt.SigningMethodPS256, jwt.SigningMethodPS384, jwt.SigningMethodPS512,
				jwt.SigningMethodRS256, jwt.SigningMethodRS384, jwt.SigningMethodRS512:
				if k, ok := tc.key.(*rsa.PrivateKey); ok {
					key = k.Public()
				}
			default:
				if k, ok := tc.key.([]byte); ok {
					key = k
				}
			}

			options := Options{
				Issuer:        "issuer",
				Subject:       "subject",
				Audience:      []string{"audience"},
				SigningMethod: tc.method,
			}

			issuer := NewIssuer(tc.key, options)
			claims := issuer.CreateToken(time.Hour, "scope1", "scope2", "scope3")

			token, err := issuer.Sign(claims)
			if err != nil {
				t.Fatal(err)
			}

			authorizer := NewAuthorizer(key, options)

			parsedClaims, err := authorizer.Validate(token)
			if err != nil {
				t.Fatal(err)
			}

			err = authorizer.Authorize(parsedClaims, claims.Scopes...)
			if err != nil {
				t.Fatal(err)
			}
		})
	}
}

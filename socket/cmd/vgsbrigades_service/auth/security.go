package auth

import (
	"errors"
	"log/slog"
	"net/http"

	swagerrs "github.com/go-openapi/errors" // for unification of swagger errors.
	"github.com/vpngen/dc-mgmt/api/vgsbrigades/gen-server/models"
	"github.com/vpngen/dc-mgmt/internal/crypto/jwtauth"
)

var (
	SwaggerErrTokenInvalid = swagerrs.New(http.StatusUnauthorized, "invalid token")
	SwaggerErrTokenExpired = swagerrs.New(http.StatusUnauthorized, "token expired")
)

func JWTValidator(logger *slog.Logger, a jwtauth.Authorizer, bearerToken string, scope []string, testing bool) (*models.Principal, error) {
	claims, err := a.Validate(bearerToken)
	if err != nil {
		logger.Error("validating token", "error", err)

		if errors.Is(err, jwtauth.ErrTokenExpired) {
			return nil, SwaggerErrTokenExpired
		}

		return nil, SwaggerErrTokenInvalid
	}

	sub, err := claims.GetSubject()
	if err != nil {
		logger.Error("getting subject", "error", err)

		return nil, SwaggerErrTokenInvalid
	}

	logger.Debug("token validated", "scopes", scope, "sub", sub)

	return &models.Principal{
		Scope: scope,
	}, nil
}

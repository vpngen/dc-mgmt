package server

import (
	"context"
	"log"
	"log/slog"
	"net/http"

	"github.com/go-openapi/errors"
	"github.com/go-openapi/loads"
	"github.com/go-openapi/runtime"
	"github.com/go-openapi/runtime/middleware"
	"github.com/golang-jwt/jwt/v5"
	"github.com/rs/cors"

	"github.com/vpngen/dc-mgmt/api/vgsocket/gen-server/models"
	"github.com/vpngen/dc-mgmt/api/vgsocket/gen-server/restapi"
	"github.com/vpngen/dc-mgmt/api/vgsocket/gen-server/restapi/operations"
	"github.com/vpngen/dc-mgmt/internal/crypto/jwtauth"
	"github.com/vpngen/dc-mgmt/socket/cmd/vgsocket_service/auth"
	"github.com/vpngen/dc-mgmt/socket/cmd/vgsocket_service/core"
	"github.com/vpngen/dc-mgmt/socket/cmd/vgsocket_service/exporter"
	"github.com/vpngen/dc-mgmt/socket/cmd/vgsocket_service/handlers"
)

// NewAPI - init API.
func NewAPI() *operations.VGSocketRealmAPI {
	swaggerSpec, err := loads.Analyzed(restapi.SwaggerJSON, "")
	if err != nil {
		log.Fatalln(err)
	}

	// create new service API
	api := operations.NewVGSocketRealmAPI(swaggerSpec)

	api.ServeError = errors.ServeError

	api.UseSwaggerUI()

	api.JSONProducer = runtime.JSONProducer()
	api.JSONConsumer = runtime.JSONConsumer()

	return api
}

// SetSecurityHandlers - set handlers for security.
func SetSecurityHandlers(ctx context.Context, opts *APIOpts, jwtMethod jwt.SigningMethod, jwtVerifier interface{}) {
	opts.API.JWTAuth = func(authHeader string, scopes []string) (*models.Principal, error) {
		authorizer := jwtauth.NewAuthorizer(jwtVerifier, auth.NewOptions(jwtMethod))

		return auth.JWTValidator(opts.Logger, authorizer, authHeader, scopes, opts.Testing)
	}
}

func SetBrigadeHandlers(ctx context.Context, opts *APIOpts) {
	hopts := &handlers.Options{
		Options: core.Options{
			Db:    opts.Db,
			SqFmt: opts.SqFmt,

			AccessKey: opts.AccessKey,

			KdTesting: opts.KdTesting,
		},

		Testing: opts.Testing,
	}

	opts.API.GetBrigadeSlotsHandler = operations.GetBrigadeSlotsHandlerFunc(func(params operations.GetBrigadeSlotsParams, principal *models.Principal) middleware.Responder {
		logger := NewLogger(opts.Logger, params.HTTPRequest, principal)

		return handlers.GetBrigadeSlotsHandler(ctx, logger, hopts, params, principal)
	})

	opts.API.GetBrigadeActivityHandler = operations.GetBrigadeActivityHandlerFunc(func(params operations.GetBrigadeActivityParams, principal *models.Principal) middleware.Responder {
		logger := NewLogger(opts.Logger, params.HTTPRequest, principal)

		return handlers.GetBrigadeActivityHandler(ctx, logger, hopts, params, principal)
	})
}

func SetUserHandlers(ctx context.Context, opts *APIOpts) {
	hopts := &handlers.Options{
		Options: core.Options{
			Db:    opts.Db,
			SqFmt: opts.SqFmt,

			AccessKey: opts.AccessKey,

			KdTesting: opts.KdTesting,
		},

		Testing: opts.Testing,
	}

	opts.API.CreateConfigHandler = operations.CreateConfigHandlerFunc(func(params operations.CreateConfigParams, principal *models.Principal) middleware.Responder {
		logger := NewLogger(opts.Logger, params.HTTPRequest, principal)

		return handlers.PostConfigHandler(ctx, logger, hopts, params, principal)
	})

	opts.API.DeleteConfigHandler = operations.DeleteConfigHandlerFunc(func(params operations.DeleteConfigParams, principal *models.Principal) middleware.Responder {
		logger := NewLogger(opts.Logger, params.HTTPRequest, principal)

		return handlers.DeleteUserHandler(ctx, logger, hopts, params, principal)
	})

	opts.API.BlockConfigHandler = operations.BlockConfigHandlerFunc(func(params operations.BlockConfigParams, principal *models.Principal) middleware.Responder {
		logger := NewLogger(opts.Logger, params.HTTPRequest, principal)

		return handlers.BlockUserHandler(ctx, logger, hopts, params, principal)
	})

	opts.API.UnblockConfigHandler = operations.UnblockConfigHandlerFunc(func(params operations.UnblockConfigParams, principal *models.Principal) middleware.Responder {
		logger := NewLogger(opts.Logger, params.HTTPRequest, principal)

		return handlers.UnblockUserHandler(ctx, logger, hopts, params, principal)
	})
}

// MakeVGSocketRealmAPIHandler - finally create a HTTP handler for the API.
func MakeVGSocketRealmAPIHandler(api *operations.VGSocketRealmAPI, pcors bool) http.Handler {
	// Important CORS part.
	switch pcors {
	case true:
		return cors.AllowAll().Handler(
			appMiddleware(api.Serve(nil)),
		)
	default:
		return appMiddleware(api.Serve(nil))
	}
}

// appMiddleware - middleware for API.
func appMiddleware(handler http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		logger := slog.Default().
			With(slog.Group("api", "method", r.Method, "endpoint", r.URL.Path)).
			With("raddr", r.RemoteAddr)

		logger.Info("request")

		if r.Method == "GET" && r.URL.Path == "/metrics/" {
			exporter.MetricsHandler(handler).ServeHTTP(w, r)

			return
		}

		handler.ServeHTTP(w, r)
	})
}

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

	"github.com/vpngen/dc-mgmt/api/vgsbrigades/gen-server/models"
	"github.com/vpngen/dc-mgmt/api/vgsbrigades/gen-server/restapi"
	"github.com/vpngen/dc-mgmt/api/vgsbrigades/gen-server/restapi/operations"
	"github.com/vpngen/dc-mgmt/internal/crypto/jwtauth"
	"github.com/vpngen/dc-mgmt/socket/cmd/vgsbrigades_service/auth"
	"github.com/vpngen/dc-mgmt/socket/cmd/vgsbrigades_service/exporter"
	"github.com/vpngen/dc-mgmt/socket/cmd/vgsbrigades_service/handlers"
)

// NewAPI - init API.
func NewAPI() *operations.VGSBrigadeRealmAPI {
	swaggerSpec, err := loads.Analyzed(restapi.SwaggerJSON, "")
	if err != nil {
		log.Fatalln(err)
	}

	// create new service API
	api := operations.NewVGSBrigadeRealmAPI(swaggerSpec)

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
		Db:    opts.Db,
		SqFmt: opts.SqFmt,

		Testing: opts.Testing,
	}

	api := opts.API

	api.CreateBrigadeHandler = operations.CreateBrigadeHandlerFunc(func(params operations.CreateBrigadeParams, principal *models.Principal) middleware.Responder {
		logger := NewLogger(opts.Logger, params.HTTPRequest, principal)

		return handlers.PostBrigadeHandler(ctx, logger, hopts, params, principal)
	})

	api.DeleteBrigadeHandler = operations.DeleteBrigadeHandlerFunc(func(params operations.DeleteBrigadeParams, principal *models.Principal) middleware.Responder {
		logger := NewLogger(opts.Logger, params.HTTPRequest, principal)

		return handlers.DeleteBrigadeHandler(ctx, logger, hopts, params, principal)
	})

	api.CheckOrderStatusHandler = operations.CheckOrderStatusHandlerFunc(func(params operations.CheckOrderStatusParams, principal *models.Principal) middleware.Responder {
		logger := NewLogger(opts.Logger, params.HTTPRequest, principal)

		return handlers.CheckOrderStatusHandler(ctx, logger, hopts, params, principal)
	})
}

// MakeVGSocketRealmAPIHandler - finally create a HTTP handler for the API.
func MakeVGSocketRealmAPIHandler(api *operations.VGSBrigadeRealmAPI, pcors bool) http.Handler {
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

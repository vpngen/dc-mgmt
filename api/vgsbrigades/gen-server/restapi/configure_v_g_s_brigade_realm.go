// This file is safe to edit. Once it exists it will not be overwritten

package restapi

import (
	"crypto/tls"
	"net/http"

	"github.com/go-openapi/errors"
	"github.com/go-openapi/runtime"
	"github.com/go-openapi/runtime/middleware"

	"github.com/vpngen/dc-mgmt/api/vgsbrigades/gen-server/models"
	"github.com/vpngen/dc-mgmt/api/vgsbrigades/gen-server/restapi/operations"
)

//go:generate swagger generate server --target ../../gen-server --name VGSBrigadeRealm --spec ../../swagger.yaml --principal models.Principal --exclude-main

func configureFlags(api *operations.VGSBrigadeRealmAPI) {
	// api.CommandLineOptionsGroups = []swag.CommandLineOptionsGroup{ ... }
}

func configureAPI(api *operations.VGSBrigadeRealmAPI) http.Handler {
	// configure the api here
	api.ServeError = errors.ServeError

	// Set your custom logger if needed. Default one is log.Printf
	// Expected interface func(string, ...interface{})
	//
	// Example:
	// api.Logger = log.Printf

	api.UseSwaggerUI()
	// To continue using redoc as your UI, uncomment the following line
	// api.UseRedoc()

	api.JSONConsumer = runtime.JSONConsumer()

	api.JSONProducer = runtime.JSONProducer()

	if api.JWTAuth == nil {
		api.JWTAuth = func(token string, scopes []string) (*models.Principal, error) {
			return nil, errors.NotImplemented("oauth2 bearer auth (JWT) has not yet been implemented")
		}
	}

	// Set your custom authorizer if needed. Default one is security.Authorized()
	// Expected interface runtime.Authorizer
	//
	// Example:
	// api.APIAuthorizer = security.Authorized()

	if api.CheckOrderStatusHandler == nil {
		api.CheckOrderStatusHandler = operations.CheckOrderStatusHandlerFunc(func(params operations.CheckOrderStatusParams, principal *models.Principal) middleware.Responder {
			return middleware.NotImplemented("operation operations.CheckOrderStatus has not yet been implemented")
		})
	}
	if api.CreateBrigadeHandler == nil {
		api.CreateBrigadeHandler = operations.CreateBrigadeHandlerFunc(func(params operations.CreateBrigadeParams, principal *models.Principal) middleware.Responder {
			return middleware.NotImplemented("operation operations.CreateBrigade has not yet been implemented")
		})
	}
	if api.DeleteBrigadeHandler == nil {
		api.DeleteBrigadeHandler = operations.DeleteBrigadeHandlerFunc(func(params operations.DeleteBrigadeParams, principal *models.Principal) middleware.Responder {
			return middleware.NotImplemented("operation operations.DeleteBrigade has not yet been implemented")
		})
	}
	if api.GetBrigadeHandler == nil {
		api.GetBrigadeHandler = operations.GetBrigadeHandlerFunc(func(params operations.GetBrigadeParams, principal *models.Principal) middleware.Responder {
			return middleware.NotImplemented("operation operations.GetBrigade has not yet been implemented")
		})
	}

	api.PreServerShutdown = func() {}

	api.ServerShutdown = func() {}

	return setupGlobalMiddleware(api.Serve(setupMiddlewares))
}

// The TLS configuration before HTTPS server starts.
func configureTLS(tlsConfig *tls.Config) {
	// Make all necessary changes to the TLS configuration here.
}

// As soon as server is initialized but not run yet, this function will be called.
// If you need to modify a config, store server instance to stop it individually later, this is the place.
// This function can be called multiple times, depending on the number of serving schemes.
// scheme value will be set accordingly: "http", "https" or "unix".
func configureServer(s *http.Server, scheme, addr string) {
}

// The middleware configuration is for the handler executors. These do not apply to the swagger.json document.
// The middleware executes after routing but before authentication, binding and validation.
func setupMiddlewares(handler http.Handler) http.Handler {
	return handler
}

// The middleware configuration happens before anything, this middleware also applies to serving the swagger.json document.
// So this is a good place to plug in a panic handling middleware, logging and metrics.
func setupGlobalMiddleware(handler http.Handler) http.Handler {
	return handler
}

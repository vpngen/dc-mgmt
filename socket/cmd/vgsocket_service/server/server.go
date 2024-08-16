package server

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"
)

// ListenAndServeHTTP starts a HTTP server.
// Actually, can be extended to support HTTPS.
func ListenAndServeHTTP(handler http.Handler, logger *slog.Logger, listen string) {
	// Create a listener for the server.
	listener, err := net.Listen("tcp", listen)
	if err != nil {
		logger.Error("can't listen", "error", err)

		os.Exit(1)
	}

	// Create a server.
	server := &http.Server{
		Handler:     handler,
		IdleTimeout: 60 * time.Minute,
	}

	// On signal, gracefully shut down the server and wait 5
	// seconds for current connections to stop.

	done := make(chan struct{})
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	// Listen for the quit signal in a goroutine and exit the
	go func() {
		<-quit
		logger.Warn("quit signal received")

		wg := sync.WaitGroup{}

		// blank for multiple servers.
		closeFunc := func(srv *http.Server) {
			defer wg.Done()

			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			srv.SetKeepAlivesEnabled(false)
			if err := srv.Shutdown(ctx); err != nil {
				slog.Error("can't gracefully shut down the server", "error", err)
			}
		}

		logger.Info("server is shutting down")

		wg.Add(1)
		go closeFunc(server)

		wg.Wait()

		if err := listener.Close(); err != nil {
			logger.Error("can't close listener", "error", err)
		}

		close(done)
	}()

	swaggerURL := fmt.Sprintf("http://127.0.0.1%s/v1/docs", listen)
	slog.Info("server is ready", "listen", listener.Addr().String(), "swaggerURL", swaggerURL)

	// Start accepting connections.
	// In goroutine because server.Serve() blocks until.
	go func() {
		if err := server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("can't serve", "error", err)
			os.Exit(1)
		}
	}()

	// All done.
	<-done
}

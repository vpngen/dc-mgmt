package exporter

import (
	"net/http"

	"github.com/VictoriaMetrics/metrics"
)

// MetricsHandler - handler for prometheus metrics.
func MetricsHandler(handler http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		metrics.WritePrometheus(w, true)
		metrics.WriteFDMetrics(w)
	})
}

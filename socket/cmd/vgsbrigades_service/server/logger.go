package server

import (
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/vpngen/dc-mgmt/api/vgsbrigades/gen-server/models"
)

// NewLogger - logger for API and Core.
// It is used by endpoint handlers for metadata access.
func NewLogger(logger *slog.Logger, r *http.Request, principal *models.Principal) *slog.Logger {
	buf := make([]byte, 8)

	ts := time.Now()
	t0 := time.Date(ts.Year(), ts.Month(), 1, 0, 0, 0, 0, time.UTC)

	binary.BigEndian.PutUint32(buf[:4], uint32(ts.Unix()-t0.Unix()))
	rand.Read(buf[4:])

	session := fmt.Sprintf("%x", buf)

	logger = logger.
		With(slog.Group("http", "session", session, "method", r.Method, "endpoint", r.URL.Path, "raddr", r.RemoteAddr))

	/*if principal != nil {
		logger = logger.
			With(
				slog.Group("auth",
					"partner", conv.UUID4(strfmt.UUID4(principal.PartnerID.String())),
					"client", swag.StringValue(principal.ClientID)),
			)
	}*/

	return logger
}

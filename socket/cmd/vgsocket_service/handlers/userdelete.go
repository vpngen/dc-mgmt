package handlers

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/go-openapi/runtime/middleware"
	"github.com/go-openapi/strfmt"
	"github.com/go-openapi/strfmt/conv"
	"github.com/go-openapi/swag"
	"github.com/google/uuid"
	"github.com/vpngen/dc-mgmt/api/vgsocket/gen-server/models"
	"github.com/vpngen/dc-mgmt/api/vgsocket/gen-server/restapi/operations"
	"github.com/vpngen/dc-mgmt/socket/cmd/vgsocket_service/core"
	"github.com/vpngen/dc-mgmt/socket/cmd/vgsocket_service/core/vpnconfig"
)

func DeleteUserHandler(ctx context.Context, logger *slog.Logger, opts *Options,
	params operations.DeleteConfigParams, principal *models.Principal,
) middleware.Responder {
	logger.Info("delete config handler")

	if opts.Testing {
		logger.Debug("delete fake config", "user_id", params.ConfigID)

		if err := vpnconfig.DeleteRandomUser(); err != nil {
			return operations.NewDeleteConfigInternalServerError()
		}

		return operations.NewDeleteConfigOK()
	}

	userID, err := uuid.Parse(params.ConfigID)
	if err != nil {
		logger.Error("parse config id error", "error", err)

		return operations.NewDeleteConfigInternalServerError()
	}

	brigadeID, err := uuid.Parse(params.Body.BrigadeID.String())
	if err != nil {
		logger.Error("parse brigade id error", "error", err)

		return operations.NewDeleteConfigInternalServerError()
	}

	slots, err := vpnconfig.DeleteUser(ctx, logger, &opts.Options, brigadeID, userID)
	if err != nil {
		if errors.Is(err, core.ErrUserNotFound) {
			logger.Warn("config not found", "user_id", params.ConfigID)

			return operations.NewDeleteConfigNotFound()
		}

		if errors.Is(err, core.ErrTemporarilyUnavailable) {
			logger.Error("temporary error deleting config", "error", err)

			retryAfter := time.Now().Add(5 * time.Minute).UTC()

			return operations.NewDeleteConfigServiceUnavailable().WithPayload(&models.ServiceTemporarilyUnavailable{
				Message:    swag.String(err.Error()),
				RetryAfter: conv.DateTime(strfmt.DateTime(retryAfter)),
			})
		}

		logger.Error("error deleting config", "error", err)

		return operations.NewDeleteConfigInternalServerError()
	}

	return operations.NewDeleteConfigOK().WithPayload(&models.FreeSlots{
		FreeSlots: swag.Int64(int64(slots)),
	})
}

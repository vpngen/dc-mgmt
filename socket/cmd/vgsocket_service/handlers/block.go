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

func BlockUserHandler(ctx context.Context, logger *slog.Logger, opts *Options,
	params operations.BlockConfigParams, principal *models.Principal,
) middleware.Responder {
	logger.Info("block config handler")

	if opts.Testing {
		logger.Debug("block fake config", "user_id", params.ConfigID)

		return operations.NewBlockConfigOK().WithPayload(&models.FreeSlots{
			FreeSlots: swag.Int64(100),
		})
	}

	userID, err := uuid.Parse(params.ConfigID)
	if err != nil {
		logger.Error("parse config id error", "error", err)

		return operations.NewBlockConfigInternalServerError()
	}

	brigadeID, err := uuid.Parse(params.Body.BrigadeID.String())
	if err != nil {
		logger.Error("parse brigade id error", "error", err)

		return operations.NewBlockConfigInternalServerError()
	}

	slots, err := vpnconfig.BlockUnblockUser(ctx, logger, &opts.Options, brigadeID, userID, true)
	if err != nil {
		if errors.Is(err, core.ErrUserNotFound) {
			logger.Warn("config not found", "user_id", params.ConfigID)

			return operations.NewBlockConfigNotFound()
		}

		if errors.Is(err, core.ErrTemporarilyUnavailable) {
			logger.Error("temporary error blocking config", "error", err)

			retryAfter := time.Now().Add(5 * time.Minute).UTC()

			return operations.NewBlockConfigServiceUnavailable().WithPayload(&models.ServiceTemporarilyUnavailable{
				Message:    swag.String(err.Error()),
				RetryAfter: conv.DateTime(strfmt.DateTime(retryAfter)),
			})
		}

		logger.Error("error blocking config", "error", err)

		return operations.NewBlockConfigInternalServerError()
	}

	return operations.NewBlockConfigOK().WithPayload(&models.FreeSlots{
		FreeSlots: swag.Int64(int64(slots)),
	})
}

func UnblockUserHandler(ctx context.Context, logger *slog.Logger, opts *Options,
	params operations.UnblockConfigParams, principal *models.Principal,
) middleware.Responder {
	logger.Info("unblock config handler")

	if opts.Testing {
		logger.Debug("unblock fake config", "user_id", params.ConfigID)

		return operations.NewUnblockConfigOK().WithPayload(&models.FreeSlots{
			FreeSlots: swag.Int64(100),
		})
	}

	userID, err := uuid.Parse(params.ConfigID)
	if err != nil {
		logger.Error("parse config id error", "error", err)

		return operations.NewUnblockConfigInternalServerError()
	}

	brigadeID, err := uuid.Parse(params.Body.BrigadeID.String())
	if err != nil {
		logger.Error("parse brigade id error", "error", err)

		return operations.NewUnblockConfigInternalServerError()
	}

	slots, err := vpnconfig.BlockUnblockUser(ctx, logger, &opts.Options, brigadeID, userID, false)
	if err != nil {
		if errors.Is(err, core.ErrUserNotFound) {
			logger.Warn("config not found", "user_id", params.ConfigID)

			return operations.NewUnblockConfigNotFound()
		}

		if errors.Is(err, core.ErrTemporarilyUnavailable) {
			logger.Error("temporary error unblocking config", "error", err)

			retryAfter := time.Now().Add(5 * time.Minute).UTC()

			return operations.NewUnblockConfigServiceUnavailable().WithPayload(&models.ServiceTemporarilyUnavailable{
				Message:    swag.String(err.Error()),
				RetryAfter: conv.DateTime(strfmt.DateTime(retryAfter)),
			})
		}

		logger.Error("error unblocking config", "error", err)

		return operations.NewUnblockConfigInternalServerError()
	}

	return operations.NewUnblockConfigOK().WithPayload(&models.FreeSlots{
		FreeSlots: swag.Int64(int64(slots)),
	})
}

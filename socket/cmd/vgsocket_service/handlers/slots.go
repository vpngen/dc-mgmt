package handlers

import (
	"context"
	"log/slog"

	"github.com/go-openapi/runtime/middleware"
	"github.com/google/uuid"
	"github.com/vpngen/dc-mgmt/api/vgsocket/gen-server/models"
	"github.com/vpngen/dc-mgmt/api/vgsocket/gen-server/restapi/operations"
	"github.com/vpngen/dc-mgmt/socket/cmd/vgsocket_service/core/brigade"
)

func GetBrigadeSlotsHandler(ctx context.Context, logger *slog.Logger, opts *Options,
	params operations.GetBrigadeSlotsParams, principal *models.Principal,
) middleware.Responder {
	logger.Info("get slots", "brigade_id", params.BrigadeID)

	brigadeID, err := uuid.Parse(params.BrigadeID)
	if err != nil {
		logger.Error("parse brigade id error", "error", err)

		return operations.NewGetBrigadeSlotsInternalServerError()
	}

	slots, err := brigade.GetSlotsInfo(ctx, logger, &opts.Options, brigadeID)
	if err != nil {
		logger.Error("get slots error", "error", err)

		return operations.NewGetBrigadeSlotsInternalServerError()
	}

	return operations.NewGetBrigadeSlotsOK().WithPayload(slots)
}

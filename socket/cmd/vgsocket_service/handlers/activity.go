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

func GetBrigadeActivityHandler(ctx context.Context, logger *slog.Logger, opts *Options,
	params operations.GetBrigadeActivityParams, principal *models.Principal,
) middleware.Responder {
	logger.Info("get slots", "brigade_id", params.BrigadeID)

	brigadeID, err := uuid.Parse(params.BrigadeID)
	if err != nil {
		logger.Error("parse brigade id error", "error", err)

		return operations.NewGetBrigadeActivityInternalServerError()
	}

	acts, err := brigade.GetActivityInfo(ctx, logger, &opts.Options, brigadeID)
	if err != nil {
		logger.Error("get activity error", "error", err)

		return operations.NewGetBrigadeActivityInternalServerError()
	}

	return operations.NewGetBrigadeActivityOK().WithPayload(acts)
}

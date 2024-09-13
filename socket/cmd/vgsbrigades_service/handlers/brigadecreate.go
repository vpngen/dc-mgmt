package handlers

import (
	"context"
	"log/slog"

	"github.com/go-openapi/runtime/middleware"
	"github.com/go-openapi/strfmt"
	"github.com/go-openapi/swag"
	"github.com/google/uuid"
	"github.com/vpngen/dc-mgmt/api/vgsbrigades/gen-server/models"
	"github.com/vpngen/dc-mgmt/api/vgsbrigades/gen-server/restapi/operations"

	dcmgmtlib "github.com/vpngen/dc-mgmt/internal/kdlib/dc-mgmt"
)

func PostBrigadeHandler(ctx context.Context, logger *slog.Logger, opts *Options,
	params operations.CreateBrigadeParams, principal *models.Principal,
) middleware.Responder {
	logger.Info("create brigade request",
		"brigade_id", params.Body.BrigadeID.String(),
		"brigade_name", params.Body.BrigadeName,
		"zone", params.Body.Zone,
	)

	brigadeID, err := uuid.Parse(params.Body.BrigadeID.String())
	if err != nil {
		logger.Error("parse brigade id error", "error", err)

		return operations.NewCreateBrigadeInternalServerError()
	}

	brigadeName := swag.StringValue(params.Body.BrigadeName)

	zone := swag.StringValue(params.Body.Zone)

	orderID, status, retryAfter, err := dcmgmtlib.VgsOrderCreateBrigade(ctx, logger, opts.Db, opts.SqFmt, brigadeID, brigadeName, zone)
	if err != nil {
		logger.Error("error creating order", "error", err)

		return operations.NewCreateBrigadeInternalServerError()
	}

	return operations.NewCreateBrigadeAccepted().WithPayload(&models.OrderStatus{
		OrderID:    strfmt.UUID(orderID.String()),
		Status:     swag.String(status),
		RetryAfter: retryAfter,
		Message:    swag.String("brigade creation order accepted"),
	})
}

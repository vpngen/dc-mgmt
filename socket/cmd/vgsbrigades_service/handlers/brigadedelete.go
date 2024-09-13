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

func DeleteBrigadeHandler(ctx context.Context, logger *slog.Logger, opts *Options,
	params operations.DeleteBrigadeParams, principal *models.Principal,
) middleware.Responder {
	logger.Info("delete brigade handler")

	brigadeID, err := uuid.Parse(params.BrigadeID)
	if err != nil {
		logger.Error("parse brigade id error", "error", err)

		return operations.NewDeleteBrigadeInternalServerError()
	}

	// 1. create order
	orderID, status, retryAfter, err := dcmgmtlib.VgsOrderDeleteBrigade(ctx, logger, opts.Db, opts.SqFmt, brigadeID)
	if err != nil {
		logger.Error("error creating order", "error", err)

		return operations.NewDeleteBrigadeInternalServerError()
	}

	return operations.NewDeleteBrigadeAccepted().WithPayload(&models.OrderStatus{
		OrderID:    strfmt.UUID(orderID.String()),
		Status:     swag.String(status),
		RetryAfter: retryAfter,
		Message:    swag.String("brigade deletion order accepted"),
	})
}

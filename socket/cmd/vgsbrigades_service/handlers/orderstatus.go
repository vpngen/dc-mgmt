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

func CheckOrderStatusHandler(ctx context.Context, logger *slog.Logger, opts *Options,
	params operations.CheckOrderStatusParams, principal *models.Principal,
) middleware.Responder {
	logger.Info("check order status handler", "order_id", params.OrderID)

	orderID, err := uuid.Parse(params.OrderID)
	if err != nil {
		logger.Error("parse order id error", "error", err)

		return operations.NewCheckOrderStatusInternalServerError()
	}

	status, retryAfter, err := dcmgmtlib.VgsCheckOrderStatus(ctx, logger, opts.Db, opts.SqFmt, orderID)
	if err != nil {
		logger.Error("error checking order status", "error", err)

		return operations.NewCheckOrderStatusInternalServerError()
	}

	return operations.NewCreateBrigadeAccepted().WithPayload(&models.OrderStatus{
		OrderID:    strfmt.UUID(orderID.String()),
		Status:     swag.String(status),
		RetryAfter: retryAfter,
		Message:    swag.String("brigade creation order accepted"),
	})
}

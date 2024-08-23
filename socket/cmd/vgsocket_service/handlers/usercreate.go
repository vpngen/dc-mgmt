package handlers

import (
	"context"
	"log/slog"

	"github.com/go-openapi/runtime/middleware"
	"github.com/go-openapi/swag"
	"github.com/google/uuid"
	"github.com/vpngen/dc-mgmt/api/vgsocket/gen-server/models"
	"github.com/vpngen/dc-mgmt/api/vgsocket/gen-server/restapi/operations"
	"github.com/vpngen/dc-mgmt/socket/cmd/vgsocket_service/core/vpnconfig"
)

func PostConfigHandler(ctx context.Context, logger *slog.Logger, opts *Options,
	params operations.CreateConfigParams, principal *models.Principal,
) middleware.Responder {
	logger.Info("create config request",
		"config_type", swag.StringValue((*string)(params.Body.ConfigType)),
		"brigade_id", params.Body.BrigadeID.String(),
	)

	if opts.Testing {
		u, _, err := vpnconfig.CreateRandomConfig(ctx, uuid.Nil, (string)(*params.Body.ConfigType))
		if err != nil {
			logger.Error("create config error", "error", err)

			return operations.NewCreateConfigInternalServerError()
		}

		logger.Info("fake config created", "user_id", u.UserID.String())

		return operations.NewCreateConfigCreated().WithPayload(u)
	}

	brigadeID, err := uuid.Parse(params.Body.BrigadeID.String())
	if err != nil {
		logger.Error("parse brigade id error", "error", err)

		return operations.NewCreateConfigInternalServerError()
	}

	conf, err := vpnconfig.GreateUser(ctx, logger, &opts.Options, brigadeID, swag.StringValue((*string)(params.Body.ConfigType)))
	if err != nil {
		logger.Error("create config error", "error", err)

		return operations.NewCreateConfigInternalServerError()
	}

	logger.Info("config created", "user_id", conf.UserID.String(), "config_name", conf.Name)

	return operations.NewCreateConfigCreated().WithPayload(conf)
}

package server

import (
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/vpngen/dc-mgmt/api/vgsbrigades/gen-server/restapi/operations"

	sq "github.com/Masterminds/squirrel"
)

type APIOpts struct {
	Logger *slog.Logger

	API *operations.VGSBrigadeRealmAPI

	Db    *pgxpool.Pool
	SqFmt sq.StatementBuilderType

	Testing bool
}

package server

import (
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/vpngen/dc-mgmt/api/vgsocket/gen-server/restapi/operations"

	sq "github.com/Masterminds/squirrel"
)

type APIOpts struct {
	Logger *slog.Logger

	API *operations.VGSocketRealmAPI

	Db    *pgxpool.Pool
	SqFmt sq.StatementBuilderType

	AccessKey string

	Testing   bool
	KdTesting bool
}

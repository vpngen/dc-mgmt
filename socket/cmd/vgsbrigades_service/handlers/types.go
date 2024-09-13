package handlers

import (
	sq "github.com/Masterminds/squirrel"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Options struct {
	Db    *pgxpool.Pool
	SqFmt sq.StatementBuilderType

	Testing bool
}

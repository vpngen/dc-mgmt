package kdlib

import (
	"fmt"

	"github.com/jackc/pgx/v5"
)

const (
	sqlGetActiveFreeSlotsNumber = "SELECT SUM(free_slots_count) FROM %s"
	sqlGetTotalFreeSlotsNumber  = "SELECT COUNT(*) FROM %s"
)

const (
	jsonActiveFreeSlotsNumber = `{
	"active_free_slots":%d
}`
	jsonTotalFreeSlotsNumber = `{
	"total_free_slots":%d
}`
)

func GetFreeSlotsNumberStatement(schema string, active bool) string {
	switch active {
	case true:
		return fmt.Sprintf(sqlGetActiveFreeSlotsNumber, (pgx.Identifier{schema, "active_pairs"}.Sanitize()))
	default:
		return fmt.Sprintf(sqlGetTotalFreeSlotsNumber, (pgx.Identifier{schema, "slots"}.Sanitize()))
	}
}

func GetFreeSlotsNumberJSONBytes(num int32, active bool) []byte {
	switch active {
	case true:
		return fmt.Appendf([]byte{}, jsonActiveFreeSlotsNumber, num)
	default:
		return fmt.Appendf([]byte{}, jsonTotalFreeSlotsNumber, num)
	}
}

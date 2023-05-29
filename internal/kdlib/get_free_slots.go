package kdlib

import (
	"fmt"

	"github.com/jackc/pgx/v5"
)

const (
	sqlGetActiveFreeSlotsNumber = "SELECT SUM(free_slots_count) FROM %s"
	sqlGetTotalFreeSlotsNumber  = "SELECT COUNT(*) FROM %s"

	sqlGetTotalAllSlotsNumber  = "SELECT COUNT(*) FROM %s"                                                               // pairs.pairs_endpoints_ipv4
	sqlGetActiveAllSlotsNumber = "SELECT COUNT(p.*) FROM %s p JOIN %s pe ON p.pair_id=pe.pair_id WHERE p.is_active=true" // pairs.pairs, pairs.pairs_endpoints_ipv4
)

const (
	jsonActiveFreeSlotsNumber = `{
		"active_free_slots":%d
	}`
	jsonTotalFreeSlotsNumber = `{
		"total_free_slots":%d
	}`
	jsonActiveAllSlotsNumber = `{
		"active_all_slots":%d
	}`
	jsonTotalAllSlotsNumber = `{
		"total_all_slots":%d
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

func GetAllSlotsNumberStatement(schema string, active bool) string {
	switch active {
	case true:
		return fmt.Sprintf(sqlGetActiveAllSlotsNumber, (pgx.Identifier{schema, "pairs.pairs"}.Sanitize()), (pgx.Identifier{schema, "pairs.pairs_endpoints_ipv4"}.Sanitize()))
	default:
		return fmt.Sprintf(sqlGetTotalAllSlotsNumber, (pgx.Identifier{schema, "pairs.pairs_endpoints_ipv4"}.Sanitize()))
	}
}

func GetAllSlotsNumberJSONBytes(num int32, active bool) []byte {
	switch active {
	case true:
		return fmt.Appendf([]byte{}, jsonActiveAllSlotsNumber, num)
	default:
		return fmt.Appendf([]byte{}, jsonTotalAllSlotsNumber, num)
	}
}

package brigade

import "time"

type BrigadeSlots struct {
	FreeSlots  int64 `json:"free_slots"`
	TotalSlots int64 `json:"total_slots"`
}

type ActivityData struct {
	LastSeen time.Time `json:"last_seen"`

	TotalTraffic   int64 `json:"total_traffic"`
	MonthlyTraffic int64 `json:"monthly_traffic"`
	PrevDayTraffic int64 `json:"prev_day_traffic"`

	Updated time.Time `json:"updated"`
}

type BrigadeActivity map[string]ActivityData

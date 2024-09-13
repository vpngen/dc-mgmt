package brigade

import "time"

type BrigadeSlots struct {
	FreeSlots  int64 `json:"free_slots"`
	TotalSlots int64 `json:"total_slots"`
}

type ActivityData struct {
	LastSeen time.Time `json:"last_seen"`
	Updated  time.Time `json:"updated"`
}

type BrigadeActivity map[string]ActivityData

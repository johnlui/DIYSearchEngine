package storage

import (
	"time"

	"github.com/johnlui/enterprise-search-engine/db"
	"github.com/johnlui/enterprise-search-engine/internal/keys"
)

func (h Handles) HostCountValues(prefix string, now time.Time) []string {
	values, _ := h.Redis.HVals(db.Ctx, keys.Day(prefix, now)).Result()
	return values
}

func (h Handles) RedisString(key, fallback string) string {
	value, err := h.Redis.Get(db.Ctx, key).Result()
	if err != nil {
		return fallback
	}
	return value
}

func (h Handles) RedisInt(key string) int {
	value, _ := h.Redis.Get(db.Ctx, key).Int()
	return value
}

func (h Handles) EstimatedStatusCount() int {
	totalCount := 0
	h.PagesDB.Raw("select count(*) from status_70").Scan(&totalCount)
	return totalCount * 256
}

func (h Handles) CrawlQueueLength() int64 {
	return h.Redis.LLen(db.Ctx, keys.CrawlQueue).Val()
}

func (h Handles) EstimatedIndexedDocumentCount() int {
	count := 0
	h.PagesDB.Raw("select count(*) from pages_70 where dic_done = 1").Scan(&count)
	return count * 256
}

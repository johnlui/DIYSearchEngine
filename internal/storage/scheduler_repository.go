package storage

import (
	"strings"
	"time"

	"github.com/go-redis/redis/v8"
	"github.com/johnlui/enterprise-search-engine/db"
	"github.com/johnlui/enterprise-search-engine/internal/crawlqueue"
	"github.com/johnlui/enterprise-search-engine/internal/indexstore"
	"github.com/johnlui/enterprise-search-engine/internal/keys"
	"github.com/johnlui/enterprise-search-engine/models"
	"gorm.io/gorm"
)

type WordAppend struct {
	Word    string
	Payload string
}

func (h Handles) SyncPagesToStatus(index int) (int64, error) {
	pagesTableName := h.PageTable(index)
	statusTableName := h.StatusTable(index)
	result := h.PagesDB.Exec("insert into `" + statusTableName + "` select `id`, `url`, `host`, `craw_done`, `craw_time` from `" + pagesTableName + "` where id > COALESCE((select max(id) from `" + statusTableName + "`), 0);")
	return result.RowsAffected, result.Error
}

func (h Handles) LoadHostBlacklist(defaultDomains map[string]struct{}) []string {
	seen := make(map[string]struct{})
	for domain := range defaultDomains {
		seen[domain] = struct{}{}
	}

	values, _ := h.Redis.SMembers(db.Ctx, keys.SpiderHostBlacklist).Result()
	for _, value := range values {
		seen[value] = struct{}{}
	}

	if len(values) == 0 {
		h.Redis.SAdd(db.Ctx, keys.SpiderHostBlacklist, "ooxx")
		h.Redis.Expire(db.Ctx, keys.SpiderHostBlacklist, time.Minute*42).Err()
	}

	result := make([]string, 0, len(seen))
	for value := range seen {
		result = append(result, value)
	}
	return result
}

func (h Handles) PopCrawlStatuses(count int) []models.Status {
	return crawlqueue.New(h.Redis, db.Ctx).PopBatch(count)
}

func (h Handles) RecordCrawlResults(results map[int]int, now time.Time) {
	pipe := h.Redis.Pipeline()
	resultKey := keys.Minute(keys.SpiderResultPerMinute, now)
	pipe.IncrBy(db.Ctx, resultKey, int64(results[1]))
	pipe.Expire(db.Ctx, resultKey, time.Hour)

	retiredKey := keys.Minute(keys.SpiderRetiredPerMinute, now)
	pipe.IncrBy(db.Ctx, retiredKey, int64(results[4]))
	pipe.Expire(db.Ctx, retiredKey, time.Hour)
	_, _ = pipe.Exec(db.Ctx)
}

func (h Handles) EnqueueStatusesForTable(index, maxNumber int, hostBlacklist []string) int {
	tableName := h.StatusTable(index)

	var statuses []models.Status
	cursorKey := keys.StatusQueueCursor(tableName)
	maxID, _ := h.Redis.Get(db.Ctx, cursorKey).Int()
	query := h.PagesDB.Table(tableName).
		Where("craw_done", models.CrawlPending).
		Where("id > ?", maxID)
	if len(hostBlacklist) > 0 {
		query = query.Where("host not in (?)", hostBlacklist)
	}
	query.
		Order("id").
		Limit(maxNumber).
		Find(&statuses)

	if len(statuses) == 0 {
		return 0
	}

	if err := crawlqueue.New(h.Redis, db.Ctx).Push(statuses); err != nil {
		return 0
	}

	keyTTL, _ := h.Redis.TTL(db.Ctx, cursorKey).Result()
	if keyTTL == -1 {
		keyTTL = time.Hour
	}

	if err := h.Redis.Set(db.Ctx, cursorKey, statuses[len(statuses)-1].ID, keyTTL).Err(); err != nil {
		return 0
	}
	return len(statuses)
}

func (h Handles) RefreshHostCountsForShard(index int, now time.Time) error {
	if err := h.refreshHostCounts(index, h.StatusTable(index), "count", "where host is not null group by host having count > 500", keys.Day(keys.HostCountsAll, now)); err != nil {
		return err
	}
	if err := h.refreshHostCounts(index, h.StatusTable(index), "crawd_count", "where craw_done = 1 and host is not null group by host", keys.Day(keys.HostCountsCrawled, now)); err != nil {
		return err
	}
	return h.refreshHostCounts(index, h.PageTable(index), "crawd_count", "where craw_done = 1 and text = '' and host is not null group by host", keys.Day(keys.HostCountsInvalid, now))
}

func (h Handles) refreshHostCounts(_ int, tableName, countColumn, whereClause, redisKey string) error {
	var counts []models.HostCount
	if err := h.PagesDB.Raw("select host, count(*) " + countColumn + " from " + tableName + " " + whereClause).Scan(&counts).Error; err != nil {
		return err
	}

	for _, value := range counts {
		count := value.Count
		if countColumn == "crawd_count" {
			count = value.CrawdCount
		}
		h.Redis.HIncrBy(db.Ctx, redisKey, value.Host, int64(count))
	}
	h.Redis.Expire(db.Ctx, redisKey, time.Hour*48).Err()
	return nil
}

func (h Handles) LoadPagesForIndex(tableName string, limit int) []models.Page {
	var pages []models.Page
	h.PagesDB.Table(tableName).
		Where("dic_done = 0").
		Where("craw_done = ?", models.CrawlDone).
		Order("id asc").
		Limit(limit).
		Scan(&pages)
	return pages
}

func (h Handles) PushIndexAppend(word, payload string) {
	h.IndexRedis.RPush(db.Ctx, word, payload)
}

func (h Handles) MarkPageIndexed(tableName string, page models.Page) {
	page.DicDone = 1
	h.PagesDB.Table(tableName).Save(&page)
}

func (h Handles) PopWordAppend(depth int64) WordAppend {
	word := h.IndexRedis.RandomKey(db.Ctx).Val()
	if word == "" {
		return WordAppend{}
	}

	listLength := h.IndexRedis.LLen(db.Ctx, word).Val()
	if listLength <= 0 {
		return WordAppend{}
	}

	if !h.Redis.HExists(db.Ctx, keys.TransferredWords, word).Val() {
		if err := indexstore.EnsureWordDic(h.DictionaryDB, word); err != nil {
			return WordAppend{}
		}
	}
	h.Redis.HSet(db.Ctx, keys.TransferredWords, word, "")

	limit := listLength
	if limit > depth {
		limit = depth
	}

	var valuesCmd *redis.StringSliceCmd
	_, _ = h.IndexRedis.TxPipelined(db.Ctx, func(pipe redis.Pipeliner) error {
		valuesCmd = pipe.LRange(db.Ctx, word, 0, limit-1)
		pipe.LTrim(db.Ctx, word, limit, -1)
		return nil
	})

	values, err := valuesCmd.Result()
	if err != nil || len(values) == 0 {
		return WordAppend{}
	}
	return WordAppend{Word: word, Payload: strings.Join(values, "")}
}

func (h Handles) SaveWordAppends(updates map[string]string) error {
	hasPostingsTable := h.DictionaryDB.Migrator().HasTable(indexstore.WordPostingsTable)
	return h.DictionaryDB.Transaction(func(tx *gorm.DB) error {
		for word, payload := range updates {
			if err := indexstore.AppendLegacyWordDic(tx, word, payload); err != nil {
				return err
			}
			if hasPostingsTable {
				if err := indexstore.AppendPostings(tx, word, payload); err != nil {
					return err
				}
			}
		}
		return nil
	})
}

func (h Handles) LoadWordBlacklist() map[string]struct{} {
	words := []string{}
	h.PagesDB.Raw("select word from word_black_list").Scan(&words)

	result := make(map[string]struct{}, len(words))
	for _, word := range words {
		result[word] = struct{}{}
	}
	return result
}

func (h Handles) LoadDomainBlacklist(base map[string]struct{}) map[string]struct{} {
	result := make(map[string]struct{}, len(base))
	for value := range base {
		result[value] = struct{}{}
	}

	domains := []string{}
	h.PagesDB.Raw("select domain from domain_black_list").Scan(&domains)
	for _, domain := range domains {
		result[domain] = struct{}{}
	}
	return result
}

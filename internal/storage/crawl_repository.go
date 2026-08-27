package storage

import (
	"fmt"
	"time"

	"github.com/go-redis/redis/v8"
	"github.com/johnlui/enterprise-search-engine/db"
	"github.com/johnlui/enterprise-search-engine/internal/keys"
	"github.com/johnlui/enterprise-search-engine/models"
)

type CrawlRateWindow struct {
	Seconds int
	Limit   int
}

type DiscoveredLink struct {
	Title   string
	URL     string
	Scheme  string
	Host    string
	Domain1 string
	Domain2 string
	Path    string
	Query   string
}

func (h Handles) ReadKVInt(key string) (int, error) {
	value := 0
	result := h.PagesDB.Table("kvstores").Where("k", key).Select("v").Find(&value)
	if result.Error != nil {
		return 0, result.Error
	}
	if result.RowsAffected == 0 {
		return 0, fmt.Errorf("kvstores key %q not found", key)
	}
	return value, nil
}

func (h Handles) CountCrawlFailure(url string) int {
	key := keys.CrawlFailureCount(url)

	count, err := h.Redis.Get(db.Ctx, key).Int()
	if err == nil && count >= 2 {
		return 4
	}

	h.Redis.IncrBy(db.Ctx, key, 1).Err()
	h.Redis.Expire(db.Ctx, key, time.Hour*240).Err()
	return 2
}

func (h Handles) HostCrawlIsLimited(host string, windows []CrawlRateWindow, now time.Time) bool {
	blocked, err := h.Redis.SIsMember(db.Ctx, keys.SpiderHostBlacklist, host).Result()
	if err == nil && blocked {
		return true
	}

	pipe := h.Redis.Pipeline()
	countCmds := make([]func() (int, error), len(windows))
	for i, window := range windows {
		cmd := pipe.Get(db.Ctx, keys.CrawlRateLimit(host, window.Seconds, now))
		countCmds[i] = cmd.Int
	}
	_, _ = pipe.Exec(db.Ctx)

	for i, getCount := range countCmds {
		count, err := getCount()
		if err == nil && count >= windows[i].Limit {
			h.AddHostToBlacklist(host)
			return true
		}
	}
	return false
}

func (h Handles) IncrementHostCrawlWindows(host string, windows []CrawlRateWindow, now time.Time) {
	pipe := h.Redis.Pipeline()
	for _, window := range windows {
		key := keys.CrawlRateLimit(host, window.Seconds, now)
		pipe.IncrBy(db.Ctx, key, 1)
		pipe.Expire(db.Ctx, key, time.Second*time.Duration(window.Seconds))
	}
	_, _ = pipe.Exec(db.Ctx)
}

func (h Handles) AddHostToBlacklist(host string) {
	h.Redis.SAdd(db.Ctx, keys.SpiderHostBlacklist, host)

	ttl, _ := h.Redis.TTL(db.Ctx, keys.SpiderHostBlacklist).Result()
	if ttl == -1 {
		h.Redis.Expire(db.Ctx, keys.SpiderHostBlacklist, time.Minute*42).Err()
	}
}

func (h Handles) SaveCrawledPage(status models.Status, title, text string) error {
	status.CrawDone = models.CrawlDone
	dbForURL := h.DBForURL(status.Url)
	if err := dbForURL.Scopes(h.StatusScope(status.Url)).Save(&status).Error; err != nil {
		return err
	}

	var page models.Page
	if err := dbForURL.Scopes(h.PageScope(status.Url)).Where(models.Page{ID: status.ID}).FirstOrCreate(&page).Error; err != nil {
		return err
	}

	page.Url = status.Url
	page.Host = status.Host
	page.CrawDone = status.CrawDone
	page.CrawTime = status.CrawTime
	page.Title = title
	page.Text = text
	return dbForURL.Scopes(h.PageScope(status.Url)).Save(&page).Error
}

func (h Handles) StatusExistenceMap(hashKey string, links []DiscoveredLink) map[string]bool {
	if len(links) == 0 {
		return map[string]bool{}
	}

	pipe := h.Redis.Pipeline()
	cmds := make([]*redis.BoolCmd, len(links))
	for i, link := range links {
		cmds[i] = pipe.HExists(db.Ctx, hashKey, link.URL)
	}
	_, _ = pipe.Exec(db.Ctx)

	result := make(map[string]bool, len(links))
	for i, cmd := range cmds {
		exists, err := cmd.Result()
		if err != nil {
			continue
		}
		result[links[i].URL] = exists
	}
	return result
}

func (h Handles) CacheKnownStatuses(hashKey string, urls []string) {
	if len(urls) == 0 {
		return
	}

	values := make([]any, 0, len(urls)*2)
	for _, url := range urls {
		values = append(values, url, 1)
	}
	h.Redis.HSet(db.Ctx, hashKey, values...).Err()
}

func (h Handles) IncrementDiscoveredStatusCounters(allCount, newCount int, now time.Time) {
	pipe := h.Redis.Pipeline()
	if allCount > 0 {
		key := keys.Minute(keys.SpiderAllURLsPerMinute, now)
		pipe.IncrBy(db.Ctx, key, int64(allCount))
		pipe.Expire(db.Ctx, key, time.Hour)
	}
	if newCount > 0 {
		key := keys.Minute(keys.SpiderNewURLsPerMinute, now)
		pipe.IncrBy(db.Ctx, key, int64(newCount))
		pipe.Expire(db.Ctx, key, time.Hour)
	}
	_, _ = pipe.Exec(db.Ctx)
}

func (h Handles) SaveDiscoveredLink(referrer models.Status, link DiscoveredLink, pendingTime time.Time) (bool, error) {
	dbForURL := h.DBForURL(link.URL)

	var newStatus models.Status
	result := dbForURL.Scopes(h.StatusScope(link.URL)).Where(models.Status{Url: link.URL}).FirstOrCreate(&newStatus)
	if result.Error != nil {
		return false, result.Error
	}

	newStatus.Url = link.URL
	newStatus.Host = link.Host
	newStatus.CrawTime = pendingTime
	if err := dbForURL.Scopes(h.StatusScope(link.URL)).Save(&newStatus).Error; err != nil {
		return false, err
	}

	var newPage models.Page
	if err := dbForURL.Scopes(h.PageScope(link.URL)).Where(models.Page{ID: newStatus.ID}).FirstOrCreate(&newPage).Error; err != nil {
		return false, err
	}

	newPage.ID = newStatus.ID
	newPage.OriginTitle = link.Title
	newPage.ReferrerId = referrer.ID
	newPage.Url = link.URL
	newPage.Scheme = link.Scheme
	newPage.Host = link.Host
	newPage.Domain1 = link.Domain1
	newPage.Domain2 = link.Domain2
	newPage.Path = link.Path
	newPage.Query = link.Query
	newPage.CrawTime = pendingTime
	if err := dbForURL.Scopes(h.PageScope(link.URL)).Save(&newPage).Error; err != nil {
		return false, err
	}

	return result.RowsAffected > 0, nil
}

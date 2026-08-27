package storage

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/go-redis/redis/v8"
	"github.com/johnlui/enterprise-search-engine/db"
	"github.com/johnlui/enterprise-search-engine/internal/indexstore"
	"github.com/johnlui/enterprise-search-engine/internal/keys"
	"github.com/johnlui/enterprise-search-engine/models"
	"github.com/johnlui/enterprise-search-engine/tools"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupSchedulerRepositoryTest(t *testing.T) (Handles, *miniredis.Miniredis) {
	t.Helper()

	dbInstance, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	dictionaryDB, err := gorm.Open(sqlite.Open("file:"+t.Name()+"_dictionary?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open dictionary sqlite: %v", err)
	}

	server := miniredis.RunT(t)
	redisClient := redis.NewClient(&redis.Options{Addr: server.Addr()})
	indexRedisClient := redis.NewClient(&redis.Options{Addr: server.Addr(), DB: 10})
	t.Cleanup(func() {
		redisClient.Close()
		indexRedisClient.Close()
	})

	return Handles{
		PagesDB:      dbInstance,
		DictionaryDB: dictionaryDB,
		Redis:        redisClient,
		IndexRedis:   indexRedisClient,
	}, server
}

type testKVStore struct {
	ID int
	K  string
	V  int
}

type testDomainBlacklist struct {
	ID     int
	Domain string
}

type testWordBlacklist struct {
	ID   int
	Word string
}

func migrateTable(t *testing.T, dbInstance *gorm.DB, table string, model any) {
	t.Helper()
	if err := dbInstance.Table(table).AutoMigrate(model); err != nil {
		t.Fatalf("migrate %s: %v", table, err)
	}
}

func TestHandleHelpersUseConfiguredHandles(t *testing.T) {
	store, _ := setupSchedulerRepositoryTest(t)

	originalPagesDB := db.DbInstance0
	originalDictionaryDB := db.DbInstanceDic
	originalRedis := db.Rdb
	originalIndexRedis := db.Rdb10
	t.Cleanup(func() {
		db.DbInstance0 = originalPagesDB
		db.DbInstanceDic = originalDictionaryDB
		db.Rdb = originalRedis
		db.Rdb10 = originalIndexRedis
	})
	db.DbInstance0 = store.PagesDB
	db.DbInstanceDic = store.DictionaryDB
	db.Rdb = store.Redis
	db.Rdb10 = store.IndexRedis

	fromGlobals := FromGlobals()
	if fromGlobals.PagesDB != store.PagesDB || fromGlobals.DictionaryDB != store.DictionaryDB ||
		fromGlobals.Redis != store.Redis || fromGlobals.IndexRedis != store.IndexRedis {
		t.Fatalf("FromGlobals() = %#v", fromGlobals)
	}

	url := "https://helpers.example/path"
	if store.DBForURL(url) != store.PagesDB {
		t.Fatal("DBForURL did not return PagesDB")
	}
	if got, want := store.StatusTableForURL(url), tools.MD5TableName("status", url); got != want {
		t.Fatalf("StatusTableForURL() = %q, want %q", got, want)
	}
	if got, want := store.PageTableForURL(url), tools.MD5TableName("pages", url); got != want {
		t.Fatalf("PageTableForURL() = %q, want %q", got, want)
	}
	if got, want := store.PageTable(2), "pages_02"; got != want {
		t.Fatalf("PageTable() = %q, want %q", got, want)
	}

	migrateTable(t, store.PagesDB, store.StatusTableForURL(url), &models.Status{})
	migrateTable(t, store.PagesDB, store.PageTableForURL(url), &models.Page{})
	if err := store.PagesDB.Scopes(store.StatusScope(url)).Create(&models.Status{ID: 7, Url: url, Host: "helpers.example"}).Error; err != nil {
		t.Fatalf("create scoped status: %v", err)
	}
	if err := store.PagesDB.Scopes(store.PageScope(url)).Create(&models.Page{ID: 7, Url: url, Host: "helpers.example"}).Error; err != nil {
		t.Fatalf("create scoped page: %v", err)
	}
	var status models.Status
	if err := store.PagesDB.Scopes(store.TableScope("status", url)).First(&status, 7).Error; err != nil {
		t.Fatalf("query scoped status: %v", err)
	}
}

func TestLoadHostBlacklistMergesRedisAndDefaultDomains(t *testing.T) {
	store, _ := setupSchedulerRepositoryTest(t)
	store.Redis.SAdd(db.Ctx, keys.SpiderHostBlacklist, "temporary.example")

	blacklist := store.LoadHostBlacklist(map[string]struct{}{
		"default.example": {},
	})

	values := make(map[string]struct{}, len(blacklist))
	for _, value := range blacklist {
		values[value] = struct{}{}
	}
	for _, want := range []string{"temporary.example", "default.example"} {
		if _, ok := values[want]; !ok {
			t.Fatalf("blacklist missing %q: %#v", want, blacklist)
		}
	}
}

func TestLoadHostBlacklistKeepsPlaceholderWhenRedisEmpty(t *testing.T) {
	store, server := setupSchedulerRepositoryTest(t)

	blacklist := store.LoadHostBlacklist(map[string]struct{}{
		"default.example": {},
	})

	if len(blacklist) != 1 || blacklist[0] != "default.example" {
		t.Fatalf("blacklist = %#v", blacklist)
	}
	member, err := server.SIsMember(keys.SpiderHostBlacklist, "ooxx")
	if err != nil {
		t.Fatalf("check placeholder host: %v", err)
	}
	if !member {
		t.Fatal("expected redis placeholder host")
	}
	if ttl := server.TTL(keys.SpiderHostBlacklist); ttl <= 0 || ttl > 42*time.Minute {
		t.Fatalf("placeholder ttl = %s", ttl)
	}
}

func TestEnqueueStatusesForTableAllowsEmptyBlacklist(t *testing.T) {
	store, _ := setupSchedulerRepositoryTest(t)
	tableName := tools.HexTableName("status", 1)
	if err := store.PagesDB.Table(tableName).AutoMigrate(&models.Status{}); err != nil {
		t.Fatalf("migrate status table: %v", err)
	}
	store.PagesDB.Table(tableName).Create(&[]models.Status{
		{ID: 1, Url: "https://a.example", Host: "a.example"},
		{ID: 2, Url: "https://b.example", Host: "b.example"},
	})

	if got := store.EnqueueStatusesForTable(1, 2, nil); got != 2 {
		t.Fatalf("EnqueueStatusesForTable() = %d, want 2", got)
	}

	payloads := store.Redis.LRange(db.Ctx, keys.CrawlQueue, 0, -1).Val()
	if len(payloads) != 2 {
		t.Fatalf("queue len = %d, want 2", len(payloads))
	}
	var status models.Status
	if err := json.Unmarshal([]byte(payloads[0]), &status); err != nil {
		t.Fatalf("unmarshal queued status: %v", err)
	}
	if status.Host == "" {
		t.Fatalf("queued status missing host: %#v", status)
	}
}

func TestEnqueueStatusesForTableSkipsFilteredAndEmptyBatches(t *testing.T) {
	store, _ := setupSchedulerRepositoryTest(t)
	tableName := tools.HexTableName("status", 8)
	migrateTable(t, store.PagesDB, tableName, &models.Status{})
	store.PagesDB.Table(tableName).Create(&[]models.Status{
		{ID: 1, Url: "https://blocked.example", Host: "blocked.example"},
		{ID: 2, Url: "https://allowed.example", Host: "allowed.example"},
	})
	cursorKey := keys.StatusQueueCursor(tableName)
	store.Redis.Set(db.Ctx, cursorKey, 0, 0)

	if got := store.EnqueueStatusesForTable(8, 2, []string{"blocked.example"}); got != 1 {
		t.Fatalf("filtered EnqueueStatusesForTable() = %d, want 1", got)
	}
	if ttl := store.Redis.TTL(db.Ctx, cursorKey).Val(); ttl <= 0 {
		t.Fatalf("cursor ttl = %s, want positive ttl", ttl)
	}
	if got := store.EnqueueStatusesForTable(8, 2, []string{"blocked.example"}); got != 0 {
		t.Fatalf("empty EnqueueStatusesForTable() = %d, want 0", got)
	}
}

func TestReadKVInt(t *testing.T) {
	store, _ := setupSchedulerRepositoryTest(t)
	migrateTable(t, store.PagesDB, "kvstores", &testKVStore{})
	store.PagesDB.Table("kvstores").Create(&testKVStore{K: "stop", V: 1})

	value, err := store.ReadKVInt("stop")
	if err != nil {
		t.Fatalf("ReadKVInt existing key: %v", err)
	}
	if value != 1 {
		t.Fatalf("ReadKVInt() = %d, want 1", value)
	}
	if _, err := store.ReadKVInt("missing"); err == nil {
		t.Fatal("expected missing key error")
	}
}

func TestCrawlRedisHelpers(t *testing.T) {
	store, server := setupSchedulerRepositoryTest(t)
	now := time.Date(2024, 1, 1, 0, 1, 30, 0, time.UTC)

	if got := store.CountCrawlFailure("https://fail.example"); got != 2 {
		t.Fatalf("first CountCrawlFailure() = %d, want 2", got)
	}
	if got := store.CountCrawlFailure("https://fail.example"); got != 2 {
		t.Fatalf("second CountCrawlFailure() = %d, want 2", got)
	}
	if got := store.CountCrawlFailure("https://fail.example"); got != 4 {
		t.Fatalf("third CountCrawlFailure() = %d, want 4", got)
	}

	windows := []CrawlRateWindow{{Seconds: 60, Limit: 1}}
	if store.HostCrawlIsLimited("limited.example", windows, now) {
		t.Fatal("host should not be limited before counters increment")
	}
	store.IncrementHostCrawlWindows("limited.example", windows, now)
	if !store.HostCrawlIsLimited("limited.example", windows, now) {
		t.Fatal("host should be limited after reaching the window limit")
	}
	if !store.HostCrawlIsLimited("limited.example", windows, now) {
		t.Fatal("blacklisted host should remain limited")
	}
	member, err := server.SIsMember(keys.SpiderHostBlacklist, "limited.example")
	if err != nil {
		t.Fatalf("read blacklist membership: %v", err)
	}
	if !member {
		t.Fatal("expected limited host in blacklist")
	}
	if ttl := server.TTL(keys.SpiderHostBlacklist); ttl <= 0 || ttl > 42*time.Minute {
		t.Fatalf("blacklist ttl = %s", ttl)
	}
}

func TestSaveCrawledPage(t *testing.T) {
	store, _ := setupSchedulerRepositoryTest(t)
	url := "https://save.example/page"
	migrateTable(t, store.PagesDB, store.StatusTableForURL(url), &models.Status{})
	migrateTable(t, store.PagesDB, store.PageTableForURL(url), &models.Page{})

	status := models.Status{ID: 9, Url: url, Host: "save.example", CrawTime: time.Date(2024, 1, 2, 3, 4, 5, 0, time.UTC)}
	if err := store.SaveCrawledPage(status, "Saved title", "Saved body"); err != nil {
		t.Fatalf("SaveCrawledPage: %v", err)
	}

	var savedStatus models.Status
	if err := store.PagesDB.Table(store.StatusTableForURL(url)).First(&savedStatus, 9).Error; err != nil {
		t.Fatalf("load saved status: %v", err)
	}
	if savedStatus.CrawDone != models.CrawlDone {
		t.Fatalf("saved status craw_done = %d", savedStatus.CrawDone)
	}
	var page models.Page
	if err := store.PagesDB.Table(store.PageTableForURL(url)).First(&page, 9).Error; err != nil {
		t.Fatalf("load saved page: %v", err)
	}
	if page.Title != "Saved title" || page.Text != "Saved body" || page.Host != "save.example" {
		t.Fatalf("saved page = %#v", page)
	}
}

func TestDiscoveredStatusRedisHelpers(t *testing.T) {
	store, _ := setupSchedulerRepositoryTest(t)
	now := time.Date(2024, 1, 1, 0, 5, 0, 0, time.UTC)
	links := []DiscoveredLink{
		{URL: "https://known.example"},
		{URL: "https://new.example"},
	}

	store.CacheKnownStatuses(keys.SpiderKnownStatuses, []string{links[0].URL})
	existence := store.StatusExistenceMap(keys.SpiderKnownStatuses, links)
	if !existence[links[0].URL] || existence[links[1].URL] {
		t.Fatalf("existence map = %#v", existence)
	}
	if empty := store.StatusExistenceMap(keys.SpiderKnownStatuses, nil); len(empty) != 0 {
		t.Fatalf("empty existence map = %#v", empty)
	}
	store.CacheKnownStatuses(keys.SpiderKnownStatuses, nil)

	store.IncrementDiscoveredStatusCounters(3, 2, now)
	if got := store.Redis.Get(db.Ctx, keys.Minute(keys.SpiderAllURLsPerMinute, now)).Val(); got != "3" {
		t.Fatalf("all url counter = %q", got)
	}
	if got := store.Redis.Get(db.Ctx, keys.Minute(keys.SpiderNewURLsPerMinute, now)).Val(); got != "2" {
		t.Fatalf("new url counter = %q", got)
	}
	store.IncrementDiscoveredStatusCounters(0, 0, now)
}

func TestSaveDiscoveredLinkCreatesStatusAndPage(t *testing.T) {
	store, _ := setupSchedulerRepositoryTest(t)
	pendingTime := time.Date(2024, 1, 3, 4, 5, 6, 0, time.UTC)
	link := DiscoveredLink{
		Title:   "Child",
		URL:     "https://child.example/path?q=1",
		Scheme:  "https",
		Host:    "child.example",
		Domain1: "example",
		Domain2: "child",
		Path:    "/path",
		Query:   "q=1",
	}
	migrateTable(t, store.PagesDB, store.StatusTableForURL(link.URL), &models.Status{})
	migrateTable(t, store.PagesDB, store.PageTableForURL(link.URL), &models.Page{})

	created, err := store.SaveDiscoveredLink(models.Status{ID: 42}, link, pendingTime)
	if err != nil {
		t.Fatalf("SaveDiscoveredLink: %v", err)
	}
	if !created {
		t.Fatal("expected first discovered link save to create a row")
	}
	created, err = store.SaveDiscoveredLink(models.Status{ID: 43}, link, pendingTime)
	if err != nil {
		t.Fatalf("SaveDiscoveredLink existing: %v", err)
	}
	if created {
		t.Fatal("expected second discovered link save to reuse existing status")
	}

	var status models.Status
	if err := store.PagesDB.Table(store.StatusTableForURL(link.URL)).Where("url = ?", link.URL).First(&status).Error; err != nil {
		t.Fatalf("load discovered status: %v", err)
	}
	var page models.Page
	if err := store.PagesDB.Table(store.PageTableForURL(link.URL)).First(&page, status.ID).Error; err != nil {
		t.Fatalf("load discovered page: %v", err)
	}
	if page.OriginTitle != "Child" || page.ReferrerId != 43 || page.Path != "/path" || page.Query != "q=1" {
		t.Fatalf("discovered page = %#v", page)
	}
}

func TestMonitoringRepository(t *testing.T) {
	store, _ := setupSchedulerRepositoryTest(t)
	now := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	migrateTable(t, store.PagesDB, "status_70", &models.Status{})
	migrateTable(t, store.PagesDB, "pages_70", &models.Page{})
	store.PagesDB.Table("status_70").Create(&[]models.Status{{ID: 1}, {ID: 2}})
	store.PagesDB.Table("pages_70").Create(&[]models.Page{{ID: 1, DicDone: 1}, {ID: 2, DicDone: 0}, {ID: 3, DicDone: 1}})

	store.Redis.HSet(db.Ctx, keys.Day(keys.HostCountsAll, now), "a.example", "3")
	values := store.HostCountValues(keys.HostCountsAll, now)
	if len(values) != 1 || values[0] != "3" {
		t.Fatalf("HostCountValues() = %#v", values)
	}
	if got := store.RedisString("missing", "fallback"); got != "fallback" {
		t.Fatalf("RedisString missing = %q", got)
	}
	store.Redis.Set(db.Ctx, "answer", "42", 0)
	if got := store.RedisString("answer", "fallback"); got != "42" {
		t.Fatalf("RedisString existing = %q", got)
	}
	if got := store.RedisInt("answer"); got != 42 {
		t.Fatalf("RedisInt() = %d, want 42", got)
	}
	store.Redis.LPush(db.Ctx, keys.CrawlQueue, "one", "two")
	if got := store.CrawlQueueLength(); got != 2 {
		t.Fatalf("CrawlQueueLength() = %d, want 2", got)
	}
	if got := store.EstimatedStatusCount(); got != 512 {
		t.Fatalf("EstimatedStatusCount() = %d, want 512", got)
	}
	if got := store.EstimatedIndexedDocumentCount(); got != 512 {
		t.Fatalf("EstimatedIndexedDocumentCount() = %d, want 512", got)
	}
}

func TestSyncPagesToStatus(t *testing.T) {
	store, _ := setupSchedulerRepositoryTest(t)
	pagesTable := store.PageTable(4)
	statusTable := store.StatusTable(4)
	migrateTable(t, store.PagesDB, pagesTable, &models.Page{})
	migrateTable(t, store.PagesDB, statusTable, &models.Status{})
	store.PagesDB.Table(pagesTable).Create(&[]models.Page{
		{ID: 1, Url: "https://one.example", Host: "one.example"},
		{ID: 2, Url: "https://two.example", Host: "two.example", CrawDone: models.CrawlDone},
	})
	store.PagesDB.Table(statusTable).Create(&models.Status{ID: 1, Url: "https://one.example", Host: "one.example"})

	rows, err := store.SyncPagesToStatus(4)
	if err != nil {
		t.Fatalf("SyncPagesToStatus: %v", err)
	}
	if rows != 1 {
		t.Fatalf("SyncPagesToStatus rows = %d, want 1", rows)
	}
	var status models.Status
	if err := store.PagesDB.Table(statusTable).First(&status, 2).Error; err != nil {
		t.Fatalf("load synced status: %v", err)
	}
	if status.CrawDone != models.CrawlDone {
		t.Fatalf("synced status = %#v", status)
	}
}

func TestSchedulerRedisQueueAndCounters(t *testing.T) {
	store, _ := setupSchedulerRepositoryTest(t)
	now := time.Date(2024, 1, 1, 0, 12, 0, 0, time.UTC)
	statuses := []models.Status{
		{ID: 1, Url: "https://one.example", Host: "one.example"},
		{ID: 2, Url: "https://two.example", Host: "two.example"},
	}
	if err := store.Redis.Del(db.Ctx, keys.CrawlQueue).Err(); err != nil {
		t.Fatalf("clear queue: %v", err)
	}
	if err := store.PushStatusesForTest(statuses); err != nil {
		t.Fatalf("push statuses: %v", err)
	}
	popped := store.PopCrawlStatuses(3)
	if len(popped) != 2 || popped[0].ID != 1 || popped[1].ID != 2 {
		t.Fatalf("PopCrawlStatuses() = %#v", popped)
	}

	store.RecordCrawlResults(map[int]int{1: 3, 4: 2}, now)
	if got := store.Redis.Get(db.Ctx, keys.Minute(keys.SpiderResultPerMinute, now)).Val(); got != "3" {
		t.Fatalf("result counter = %q", got)
	}
	if got := store.Redis.Get(db.Ctx, keys.Minute(keys.SpiderRetiredPerMinute, now)).Val(); got != "2" {
		t.Fatalf("retired counter = %q", got)
	}
}

func (h Handles) PushStatusesForTest(statuses []models.Status) error {
	payloads := make([]any, 0, len(statuses))
	for _, status := range statuses {
		payload, err := json.Marshal(status)
		if err != nil {
			return err
		}
		payloads = append(payloads, payload)
	}
	return h.Redis.LPush(db.Ctx, keys.CrawlQueue, payloads...).Err()
}

func TestRefreshHostCountsForShard(t *testing.T) {
	store, _ := setupSchedulerRepositoryTest(t)
	now := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	statusTable := store.StatusTable(5)
	pageTable := store.PageTable(5)
	migrateTable(t, store.PagesDB, statusTable, &models.Status{})
	migrateTable(t, store.PagesDB, pageTable, &models.Page{})

	statusRows := make([]models.Status, 0, 503)
	for i := 1; i <= 501; i++ {
		statusRows = append(statusRows, models.Status{ID: uint(i), Host: "many.example"})
	}
	statusRows = append(statusRows,
		models.Status{ID: 600, Host: "done.example", CrawDone: models.CrawlDone},
		models.Status{ID: 601, Host: "done.example", CrawDone: models.CrawlDone},
	)
	store.PagesDB.Table(statusTable).CreateInBatches(statusRows, 100)
	store.PagesDB.Table(pageTable).Create(&[]models.Page{
		{ID: 1, Host: "invalid.example", CrawDone: models.CrawlDone, Text: ""},
		{ID: 2, Host: "invalid.example", CrawDone: models.CrawlDone, Text: ""},
		{ID: 3, Host: "valid.example", CrawDone: models.CrawlDone, Text: "body"},
	})

	if err := store.RefreshHostCountsForShard(5, now); err != nil {
		t.Fatalf("RefreshHostCountsForShard: %v", err)
	}
	if got := store.Redis.HGet(db.Ctx, keys.Day(keys.HostCountsAll, now), "many.example").Val(); got != "501" {
		t.Fatalf("all host count = %q", got)
	}
	if got := store.Redis.HGet(db.Ctx, keys.Day(keys.HostCountsCrawled, now), "done.example").Val(); got != "2" {
		t.Fatalf("crawled host count = %q", got)
	}
	if got := store.Redis.HGet(db.Ctx, keys.Day(keys.HostCountsInvalid, now), "invalid.example").Val(); got != "2" {
		t.Fatalf("invalid host count = %q", got)
	}
}

func TestRefreshHostCountsForShardReturnsQueryErrors(t *testing.T) {
	store, _ := setupSchedulerRepositoryTest(t)
	now := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

	if err := store.RefreshHostCountsForShard(9, now); err == nil {
		t.Fatal("expected missing status table to fail")
	}

	migrateTable(t, store.PagesDB, store.StatusTable(10), &models.Status{})
	if err := store.RefreshHostCountsForShard(10, now); err == nil {
		t.Fatal("expected missing page table to fail")
	}
}

func TestIndexingRepository(t *testing.T) {
	store, _ := setupSchedulerRepositoryTest(t)
	tableName := store.PageTable(6)
	migrateTable(t, store.PagesDB, tableName, &models.Page{})
	migrateTable(t, store.DictionaryDB, "word_dics", &models.WordDic{})
	if err := store.DictionaryDB.AutoMigrate(&models.WordPosting{}); err != nil {
		t.Fatalf("migrate word postings: %v", err)
	}
	store.PagesDB.Table(tableName).Create(&[]models.Page{
		{ID: 1, Url: "https://ready.example", CrawDone: models.CrawlDone, DicDone: 0},
		{ID: 2, Url: "https://indexed.example", CrawDone: models.CrawlDone, DicDone: 1},
		{ID: 3, Url: "https://pending.example", CrawDone: models.CrawlPending, DicDone: 0},
	})

	pages := store.LoadPagesForIndex(tableName, 10)
	if len(pages) != 1 || pages[0].ID != 1 {
		t.Fatalf("LoadPagesForIndex() = %#v", pages)
	}
	store.MarkPageIndexed(tableName, pages[0])
	var indexed models.Page
	if err := store.PagesDB.Table(tableName).First(&indexed, 1).Error; err != nil {
		t.Fatalf("load indexed page: %v", err)
	}
	if indexed.DicDone != 1 {
		t.Fatalf("indexed page dic_done = %d", indexed.DicDone)
	}

	if empty := store.PopWordAppend(10); empty != (WordAppend{}) {
		t.Fatalf("empty PopWordAppend() = %#v", empty)
	}
	store.IndexRedis.Set(db.Ctx, "not-a-list", "value", 0)
	if empty := store.PopWordAppend(10); empty != (WordAppend{}) {
		t.Fatalf("non-list PopWordAppend() = %#v", empty)
	}
	store.IndexRedis.Del(db.Ctx, "not-a-list")
	store.PushIndexAppend("term", "6,1,2,20,0-")
	store.PushIndexAppend("term", "6,2,1,30,3-")
	appendOne := store.PopWordAppend(1)
	if appendOne.Word != "term" || appendOne.Payload != "6,1,2,20,0-" {
		t.Fatalf("PopWordAppend depth 1 = %#v", appendOne)
	}
	appendRest := store.PopWordAppend(10)
	if appendRest.Word != "term" || appendRest.Payload != "6,2,1,30,3-" {
		t.Fatalf("PopWordAppend rest = %#v", appendRest)
	}
	if err := store.SaveWordAppends(map[string]string{"term": appendOne.Payload + appendRest.Payload}); err != nil {
		t.Fatalf("SaveWordAppends: %v", err)
	}
	var dic models.WordDic
	if err := store.DictionaryDB.Table("word_dics").Where("name = ?", "term").First(&dic).Error; err != nil {
		t.Fatalf("load word dic: %v", err)
	}
	if dic.Positions != "6,1,2,20,0-6,2,1,30,3-" {
		t.Fatalf("word dic positions = %q", dic.Positions)
	}
	postings, err := indexstore.LoadPostings(store.DictionaryDB, []string{"term"})
	if err != nil {
		t.Fatalf("load postings: %v", err)
	}
	if len(postings["term"]) != 2 {
		t.Fatalf("postings = %#v", postings["term"])
	}
}

func TestSaveWordAppendsWithoutPostingsTable(t *testing.T) {
	store, _ := setupSchedulerRepositoryTest(t)
	migrateTable(t, store.DictionaryDB, "word_dics", &models.WordDic{})
	if err := indexstore.EnsureWordDic(store.DictionaryDB, "legacy"); err != nil {
		t.Fatalf("ensure legacy word dic: %v", err)
	}

	if err := store.SaveWordAppends(map[string]string{"legacy": "0,1,1,10,0-"}); err != nil {
		t.Fatalf("SaveWordAppends without postings table: %v", err)
	}
	var dic models.WordDic
	if err := store.DictionaryDB.Table("word_dics").Where("name = ?", "legacy").First(&dic).Error; err != nil {
		t.Fatalf("load legacy word dic: %v", err)
	}
	if dic.Positions != "0,1,1,10,0-" {
		t.Fatalf("legacy positions = %q", dic.Positions)
	}
}

func TestLoadBlacklists(t *testing.T) {
	store, _ := setupSchedulerRepositoryTest(t)
	migrateTable(t, store.PagesDB, "word_black_list", &testWordBlacklist{})
	migrateTable(t, store.PagesDB, "domain_black_list", &testDomainBlacklist{})
	store.PagesDB.Table("word_black_list").Create(&[]testWordBlacklist{{Word: "skip"}, {Word: "drop"}})
	store.PagesDB.Table("domain_black_list").Create(&[]testDomainBlacklist{{Domain: "db.example"}})

	words := store.LoadWordBlacklist()
	if _, ok := words["skip"]; !ok {
		t.Fatalf("word blacklist = %#v", words)
	}
	domains := store.LoadDomainBlacklist(map[string]struct{}{"base.example": {}})
	for _, want := range []string{"base.example", "db.example"} {
		if _, ok := domains[want]; !ok {
			t.Fatalf("domain blacklist missing %q: %#v", want, domains)
		}
	}
}

func TestSearchRepository(t *testing.T) {
	store, _ := setupSchedulerRepositoryTest(t)
	migrateTable(t, store.DictionaryDB, "word_dics", &models.WordDic{})
	if err := store.DictionaryDB.AutoMigrate(&models.WordPosting{}); err != nil {
		t.Fatalf("migrate word postings: %v", err)
	}
	migrateTable(t, store.PagesDB, store.PageTable(2), &models.Page{})
	store.DictionaryDB.Table("word_dics").Create(&[]models.WordDic{
		{Name: "alpha", Positions: "legacy"},
		{Name: "beta", Positions: "legacy"},
	})
	if err := indexstore.AppendPostings(store.DictionaryDB, "alpha", "2,11,4,40,1-"); err != nil {
		t.Fatalf("append postings: %v", err)
	}
	store.PagesDB.Table(store.PageTable(2)).Create(&[]models.Page{
		{ID: 11, Url: "https://alpha.example"},
		{ID: 12, Url: "https://beta.example"},
	})

	if got := store.LoadWordDics(nil); len(got) != 0 {
		t.Fatalf("LoadWordDics empty = %#v", got)
	}
	dics := store.LoadWordDics([]string{"alpha", "missing"})
	if dics["alpha"].Positions != "legacy" {
		t.Fatalf("LoadWordDics() = %#v", dics)
	}
	postings := store.LoadPostings([]string{"alpha"})
	if len(postings["alpha"]) != 1 || postings["alpha"][0].DocID != 11 {
		t.Fatalf("LoadPostings() = %#v", postings)
	}
	pages := store.LoadPagesByTableIDs(map[int][]uint{2: {11, 12}})
	for _, id := range []uint{11, 12} {
		key := fmt.Sprintf("2-%d", id)
		if pages[key].ID != id {
			t.Fatalf("LoadPagesByTableIDs missing %s: %#v", key, pages)
		}
	}
}

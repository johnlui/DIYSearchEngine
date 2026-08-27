package storage

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/go-redis/redis/v8"
	"github.com/johnlui/enterprise-search-engine/db"
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

	server := miniredis.RunT(t)
	redisClient := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() {
		redisClient.Close()
	})

	return Handles{
		PagesDB: dbInstance,
		Redis:   redisClient,
	}, server
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

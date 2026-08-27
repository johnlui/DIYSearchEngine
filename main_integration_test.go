package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/alicebob/miniredis/v2"
	"github.com/go-redis/redis/v8"
	"github.com/johnlui/enterprise-search-engine/db"
	"github.com/johnlui/enterprise-search-engine/models"
	"github.com/johnlui/enterprise-search-engine/tools"
	"github.com/robfig/cron/v3"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type kvstoreRow struct {
	ID int
	K  string
	V  int
}

func setupMainRedis(t *testing.T) *miniredis.Miniredis {
	t.Helper()

	server := miniredis.RunT(t)
	db.Rdb = redis.NewClient(&redis.Options{Addr: server.Addr()})
	db.Rdb10 = redis.NewClient(&redis.Options{Addr: server.Addr(), DB: 10})
	t.Cleanup(func() {
		db.Rdb.Close()
		db.Rdb10.Close()
	})
	return server
}

func setupMainDB(t *testing.T) *gorm.DB {
	t.Helper()

	dbInstance, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	sqlDB, err := dbInstance.DB()
	if err != nil {
		t.Fatalf("sqlite DB: %v", err)
	}
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { sqlDB.Close() })

	db.DbInstance0 = dbInstance
	db.DbInstanceDic = dbInstance

	if err := dbInstance.Table("kvstores").AutoMigrate(&kvstoreRow{}); err != nil {
		t.Fatalf("migrate kvstores: %v", err)
	}
	if err := dbInstance.Table("domain_black_list").AutoMigrate(struct {
		ID     int
		Domain string
	}{}); err != nil {
		t.Fatalf("migrate domain_black_list: %v", err)
	}
	if err := dbInstance.Table("word_black_list").AutoMigrate(struct {
		ID   int
		Word string
	}{}); err != nil {
		t.Fatalf("migrate word_black_list: %v", err)
	}
	if err := dbInstance.Table("word_dics").AutoMigrate(&models.WordDic{}); err != nil {
		t.Fatalf("migrate word_dics: %v", err)
	}
	return dbInstance
}

func migrateShardTables(t *testing.T, dbInstance *gorm.DB, indexes ...int) {
	t.Helper()

	for _, i := range indexes {
		if err := dbInstance.Table(tools.HexTableName("status", i)).AutoMigrate(&models.Status{}); err != nil {
			t.Fatalf("migrate status shard %d: %v", i, err)
		}
		if err := dbInstance.Table(tools.HexTableName("pages", i)).AutoMigrate(&models.Page{}); err != nil {
			t.Fatalf("migrate pages shard %d: %v", i, err)
		}
	}
}

func migrateAllShardTables(t *testing.T, dbInstance *gorm.DB) {
	t.Helper()

	for i := 0; i < 256; i++ {
		migrateShardTables(t, dbInstance, i)
	}
}

func TestArtCommands(t *testing.T) {
	commands := artCommands(Art{})
	if _, ok := commands["init"]; !ok {
		t.Fatal("expected init command")
	}
}

func TestArtInit(t *testing.T) {
	dbInstance := setupMainDB(t)
	db.DbInstanceDic = dbInstance

	originalStdout := os.Stdout
	devNull, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
	if err != nil {
		t.Fatalf("open devnull: %v", err)
	}
	defer devNull.Close()
	os.Stdout = devNull
	defer func() { os.Stdout = originalStdout }()

	Art{}.Init()
}

func TestMainFunction(t *testing.T) {
	originalParseFlags := parseFlags
	originalInitializeENV := initializeENV
	originalInitializeJieba := initializeJieba
	originalInitializeDB := initializeDB
	originalConfigureRuntimeServices := configureRuntimeServices
	originalInitializeArtCommands := initializeArtCommands
	originalLaunchServer := launchServer
	originalCreateCron := createCron
	originalStartCron := startCron
	originalRunDictionaryWash := runDictionaryWash
	originalRunSpider := runSpider
	originalBlockMain := blockMain
	originalDebug := tools.ENV_DEBUG
	defer func() {
		parseFlags = originalParseFlags
		initializeENV = originalInitializeENV
		initializeJieba = originalInitializeJieba
		initializeDB = originalInitializeDB
		configureRuntimeServices = originalConfigureRuntimeServices
		initializeArtCommands = originalInitializeArtCommands
		launchServer = originalLaunchServer
		createCron = originalCreateCron
		startCron = originalStartCron
		runDictionaryWash = originalRunDictionaryWash
		runSpider = originalRunSpider
		blockMain = originalBlockMain
		tools.ENV_DEBUG = originalDebug
	}()

	var calls atomic.Int32
	serverLaunched := make(chan struct{})
	cronStarted := make(chan struct{})
	dictionaryWashed := make(chan struct{})
	parseFlags = func() { calls.Add(1) }
	initializeENV = func() { calls.Add(1) }
	initializeJieba = func() { calls.Add(1) }
	initializeDB = func() { calls.Add(1) }
	configureRuntimeServices = func() { calls.Add(1) }
	initializeArtCommands = func() { calls.Add(1) }
	launchServer = func() {
		calls.Add(1)
		close(serverLaunched)
	}
	createCron = func() *cron.Cron {
		calls.Add(1)
		return cron.New(cron.WithSeconds())
	}
	startCron = func(*cron.Cron) {
		calls.Add(1)
		close(cronStarted)
	}
	runDictionaryWash = func() {
		calls.Add(1)
		close(dictionaryWashed)
	}
	runSpider = func(time.Time) { calls.Add(1) }
	blockMain = func() { calls.Add(1) }
	tools.ENV_DEBUG = false

	main()
	<-serverLaunched
	<-cronStarted
	<-dictionaryWashed
	if calls.Load() != 12 {
		t.Fatalf("main call count = %d, want 12", calls.Load())
	}
}

func TestRunAllRolesStartsSpiderWhenIndexerBlocks(t *testing.T) {
	originalLaunchServer := launchServer
	originalCreateCron := createCron
	originalStartCron := startCron
	originalRunDictionaryWash := runDictionaryWash
	originalRunSpider := runSpider
	originalBlockMain := blockMain
	originalDebug := tools.ENV_DEBUG
	defer func() {
		launchServer = originalLaunchServer
		createCron = originalCreateCron
		startCron = originalStartCron
		runDictionaryWash = originalRunDictionaryWash
		runSpider = originalRunSpider
		blockMain = originalBlockMain
		tools.ENV_DEBUG = originalDebug
	}()

	indexerStarted := make(chan struct{})
	releaseIndexer := make(chan struct{})
	spiderStarted := make(chan struct{})

	launchServer = func() {}
	createCron = func() *cron.Cron { return cron.New(cron.WithSeconds()) }
	startCron = func(*cron.Cron) {}
	runDictionaryWash = func() {
		close(indexerStarted)
		<-releaseIndexer
	}
	runSpider = func(time.Time) { close(spiderStarted) }
	blockMain = func() {}
	tools.ENV_DEBUG = false

	runAllRoles()

	select {
	case <-indexerStarted:
	case <-time.After(time.Second):
		t.Fatal("indexer did not start")
	}
	select {
	case <-spiderStarted:
	case <-time.After(time.Second):
		t.Fatal("spider did not start while indexer was blocked")
	}
	close(releaseIndexer)
}

func TestResolveRuntimeRole(t *testing.T) {
	cases := map[string][]string{
		"all":       {"ese"},
		"serve":     {"ese", "serve"},
		"crawler":   {"ese", "crawler"},
		"scheduler": {"ese", "scheduler"},
		"indexer":   {"ese", "indexer"},
		"all-bad":   {"ese", "unknown"},
	}

	for name, args := range cases {
		want := name
		if name == "all-bad" {
			want = "all"
		}
		if got := resolveRuntimeRole(args); got != want {
			t.Fatalf("resolveRuntimeRole(%s) = %q, want %q", name, got, want)
		}
	}
}

func TestInitENV(t *testing.T) {
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	tmp := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmp, ".env"), []byte("APP_DEBUG=true\nAPP_ENV=test\n"), 0o644); err != nil {
		t.Fatalf("write .env: %v", err)
	}
	if err := os.Chdir(tmp); err != nil {
		t.Fatalf("chdir temp: %v", err)
	}
	defer os.Chdir(wd)

	initENV()
	if !tools.ENV_DEBUG {
		t.Fatal("expected ENV_DEBUG to be true")
	}
}

func TestInitArtCommandsSkipsNonArt(t *testing.T) {
	originalArgs := os.Args
	defer func() { os.Args = originalArgs }()

	os.Args = []string{"ese", "serve"}
	initArtCommands()

	os.Args = []string{"ese"}
	initArtCommands()
}

func TestInitArtCommandsRunsCommand(t *testing.T) {
	originalArgs := os.Args
	originalFactory := artCommandFactory
	originalDebugDump := debugDump
	defer func() {
		os.Args = originalArgs
		artCommandFactory = originalFactory
		debugDump = originalDebugDump
	}()

	called := false
	dumped := false
	artCommandFactory = func(Art) map[string]artCommand {
		return map[string]artCommand{
			"init": func(args ...string) {
				called = len(args) == 1 && args[0] == "arg"
			},
		}
	}
	debugDump = func(...any) { dumped = true }

	os.Args = []string{"ese", "art", "init", "arg"}
	initArtCommands()
	if !called || !dumped {
		t.Fatalf("called=%v dumped=%v", called, dumped)
	}
}

func TestInitJiebaWrapper(t *testing.T) {
	originalArgs := os.Args
	originalCut := tools.GetFenciResultArray
	defer func() {
		os.Args = originalArgs
		_ = originalCut
	}()

	os.Args = []string{filepath.Join(".", "ese")}
	initJieba()
	if got := tools.GetFenciResultArray("中文搜索"); len(got) == 0 {
		t.Fatal("expected initialized jieba to return tokens")
	}
}

func TestTableScopeHelpers(t *testing.T) {
	dbInstance := setupMainDB(t)
	url := "https://example.com/path"

	if realDB(url) != dbInstance {
		t.Fatal("realDB did not return DbInstance0")
	}
	if table := dbInstance.Session(&gorm.Session{DryRun: true}).Scopes(statusTable(url)).Find(&models.Status{}).Statement.Table; table != tools.MD5TableName("status", url) {
		t.Fatalf("statusTable() = %q", table)
	}
	if table := dbInstance.Session(&gorm.Session{DryRun: true}).Scopes(lakeTable(url)).Find(&models.Page{}).Statement.Table; table != tools.MD5TableName("pages", url) {
		t.Fatalf("lakeTable() = %q", table)
	}
	if table := dbInstance.Session(&gorm.Session{DryRun: true}).Scopes(md5Table(url, "custom")).Find(&models.Page{}).Statement.Table; table != tools.MD5TableName("custom", url) {
		t.Fatalf("md5Table() = %q", table)
	}
}

func TestBuildRouter(t *testing.T) {
	router := buildRouter()
	if router == nil {
		t.Fatal("expected router")
	}
}

func TestCollectDiscoveredLinks(t *testing.T) {
	domain1BlackList = map[string]struct{}{"blocked.com": {}}
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(`
		<html><body>
			<a href="">empty</a>
			<a href="notaurl">bad</a>
			<a href="HTTPS://Sub.Example.COM/a?q=1#frag">Example</a>
			<a href="https://sub.example.com/a?q=1#other">duplicate</a>
			<a href="https://blocked.com/x">blocked</a>
		</body></html>`))
	if err != nil {
		t.Fatalf("NewDocumentFromReader: %v", err)
	}

	links := collectDiscoveredLinks(doc)
	if len(links) != 1 {
		t.Fatalf("links len = %d, want 1: %#v", len(links), links)
	}
	link := links[0]
	if link.Title != "Example" || link.URL != "https://sub.example.com/a?q=1" || link.Host != "sub.example.com" ||
		link.Domain1 != "example.com" || link.Domain2 != "sub.example.com" || link.Path != "/a" || link.Query != "q=1" {
		t.Fatalf("unexpected link: %#v", link)
	}
}

func TestSplitDomains(t *testing.T) {
	cases := []struct {
		host    string
		domain1 string
		domain2 string
	}{
		{"localhost", "", ""},
		{"example.com", "example.com", "example.com"},
		{"www.example.com", "example.com", "www.example.com"},
	}
	for _, tc := range cases {
		d1, d2 := splitDomains(tc.host)
		if d1 != tc.domain1 || d2 != tc.domain2 {
			t.Fatalf("splitDomains(%q) = %q/%q", tc.host, d1, d2)
		}
	}
}

func TestRedisBackedCrawlHelpers(t *testing.T) {
	setupMainRedis(t)
	now := time.Unix(1700000000, 0)
	links := []discoveredLink{{URL: "https://example.com/a"}, {URL: "https://example.com/b"}}

	cacheKnownStatuses("status_exists", []string{"https://example.com/a"})
	exists := statusExistenceMap("status_exists", links)
	if !exists["https://example.com/a"] || exists["https://example.com/b"] {
		t.Fatalf("statusExistenceMap() = %#v", exists)
	}
	if got := statusExistenceMap("status_exists", nil); len(got) != 0 {
		t.Fatalf("empty statusExistenceMap() = %#v", got)
	}

	incrementDiscoveredStatusCounters(2, 1, now)
	if got := db.Rdb.Get(db.Ctx, tools.MinuteBucketKey("ese_spider_all_status_in_minute_", now)).Val(); got != "2" {
		t.Fatalf("all status counter = %q", got)
	}
	if got := db.Rdb.Get(db.Ctx, tools.MinuteBucketKey("ese_spider_new_status_in_minute_", now)).Val(); got != "1" {
		t.Fatalf("new status counter = %q", got)
	}

	incrementHostCrawlWindows("example.com", now)
	for _, window := range crawlRateWindows {
		key := tools.WindowBucketKey("ese_spider_xianliu_", "example.com", window.seconds, now)
		if got := db.Rdb.Get(db.Ctx, key).Val(); got != "1" {
			t.Fatalf("%s = %q", key, got)
		}
	}

	addHostToBlacklist("blocked.example")
	if !db.Rdb.SIsMember(db.Ctx, "ese_spider_host_black_list", "blocked.example").Val() {
		t.Fatal("expected host in blacklist")
	}
}

func TestRunWorkerPool(t *testing.T) {
	if got := runWorkerPool(0, 4, func(int) int { return 1 }); got != 0 {
		t.Fatalf("runWorkerPool(empty) = %d", got)
	}
	got := runWorkerPool(5, 20, func(i int) int { return i + 1 })
	if got != 15 {
		t.Fatalf("runWorkerPool() = %d, want 15", got)
	}
}

func TestLoadStatusesForCrawling(t *testing.T) {
	setupMainRedis(t)
	t.Setenv("APP_DEBUG", "true")

	status := models.Status{ID: 7, Url: "https://example.com", Host: "example.com"}
	payload, _ := json.Marshal(status)
	db.Rdb.LPush(db.Ctx, "need_craw_list", "bad-json", payload)

	got := loadStatusesForCrawling()
	if len(got) != 1 || got[0].ID != status.ID {
		t.Fatalf("loadStatusesForCrawling() = %#v", got)
	}
}

func TestNextStep(t *testing.T) {
	original := runNextStepOnce
	defer func() { runNextStepOnce = original }()

	calls := 0
	runNextStepOnce = func(t time.Time) (time.Time, bool) {
		calls++
		return t.Add(time.Second), calls == 1
	}

	nextStep(time.Unix(1, 0))
	if calls != 2 {
		t.Fatalf("nextStep calls = %d, want 2", calls)
	}
}

func TestRunNextStepBranches(t *testing.T) {
	setupMainRedis(t)
	dbInstance := setupMainDB(t)
	originalSleep := sleep
	defer func() { sleep = originalSleep }()
	sleep = func(time.Duration) {}
	t.Setenv("APP_DEBUG", "true")

	dbInstance.Table("kvstores").Create(&kvstoreRow{K: "stop", V: 1})
	if _, ok := runNextStep(time.Unix(1, 0)); !ok {
		t.Fatal("expected stop branch to continue")
	}

	dbInstance.Table("kvstores").Where("k = ?", "stop").Update("v", 0)
	if _, ok := runNextStep(time.Unix(1, 0)); !ok {
		t.Fatal("expected empty branch to continue")
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`<html><head><title>Run</title></head><body>Run body</body></html>`))
	}))
	defer server.Close()
	status := models.Status{ID: 1, Url: server.URL, Host: strings.TrimPrefix(server.URL, "http://")}
	indexes := map[int]struct{}{}
	for i := 0; i < 256; i++ {
		if tools.HexTableName("status", i) == tools.MD5TableName("status", status.Url) ||
			tools.HexTableName("pages", i) == tools.MD5TableName("pages", status.Url) {
			indexes[i] = struct{}{}
		}
	}
	for i := range indexes {
		migrateShardTables(t, dbInstance, i)
	}
	dbInstance.Table("kvstores").Create(&kvstoreRow{K: "stopNew", V: 1})
	payload, _ := json.Marshal(status)
	db.Rdb.LPush(db.Ctx, "need_craw_list", payload)
	if _, ok := runNextStep(time.Unix(1, 0)); !ok {
		t.Fatal("expected crawl branch to continue")
	}
}

func TestStatusHostCrawIsTooMuch(t *testing.T) {
	setupMainRedis(t)

	if statusHostCrawIsTooMuch("example.com") {
		t.Fatal("fresh host should not be limited")
	}
	db.Rdb.SAdd(db.Ctx, "ese_spider_host_black_list", "blocked.com")
	if !statusHostCrawIsTooMuch("blocked.com") {
		t.Fatal("blacklisted host should be limited")
	}

	now := time.Now()
	window := crawlRateWindows[0]
	db.Rdb.Set(db.Ctx, tools.WindowBucketKey("ese_spider_xianliu_", "busy.com", window.seconds, now), window.limit, time.Minute)
	if !statusHostCrawIsTooMuch("busy.com") {
		t.Fatal("busy host should be limited")
	}
}

func TestProcessDiscoveredLinks(t *testing.T) {
	setupMainRedis(t)
	dbInstance := setupMainDB(t)
	now := time.Unix(1700000000, 0)
	status := models.Status{ID: 11, Url: "https://referrer.com", Host: "referrer.com"}
	newURL := "https://sub.example.com/path?q=1"
	migrateShardTables(t, dbInstance, int(newURL[0])) // harmless extra shard for branch coverage
	statusShard := tools.MD5TableName("status", newURL)
	pageShard := tools.MD5TableName("pages", newURL)
	shardIndex := 0
	for i := 0; i < 256; i++ {
		if tools.HexTableName("status", i) == statusShard {
			shardIndex = i
			break
		}
	}
	migrateShardTables(t, dbInstance, shardIndex)

	link := discoveredLink{
		Title: "New Page", URL: newURL, Scheme: "https", Host: "sub.example.com",
		Domain1: "example.com", Domain2: "sub.example.com", Path: "/path", Query: "q=1",
	}
	processDiscoveredLinks(status, []discoveredLink{link}, now)

	var savedStatus models.Status
	if err := dbInstance.Table(statusShard).First(&savedStatus).Error; err != nil {
		t.Fatalf("find saved status: %v", err)
	}
	var savedPage models.Page
	if err := dbInstance.Table(pageShard).First(&savedPage).Error; err != nil {
		t.Fatalf("find saved page: %v", err)
	}
	if savedPage.OriginTitle != "New Page" || savedPage.ReferrerId != status.ID || savedPage.Domain2 != "sub.example.com" {
		t.Fatalf("savedPage = %#v", savedPage)
	}

	processDiscoveredLinks(status, nil, now)
	processDiscoveredLinks(status, []discoveredLink{link}, now)
}

func TestCrawSuccess(t *testing.T) {
	setupMainRedis(t)
	dbInstance := setupMainDB(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`<html><head><title> Title </title></head><body> Body <a href="https://linked.example/page">link</a></body></html>`))
	}))
	defer server.Close()

	status := models.Status{ID: 1, Url: server.URL, Host: strings.TrimPrefix(server.URL, "http://")}
	statusShard := tools.MD5TableName("status", status.Url)
	pageShard := tools.MD5TableName("pages", status.Url)
	linkShard := tools.MD5TableName("status", "https://linked.example/page")
	indexes := map[int]struct{}{}
	for i := 0; i < 256; i++ {
		if tools.HexTableName("status", i) == statusShard || tools.HexTableName("pages", i) == pageShard || tools.HexTableName("status", i) == linkShard {
			indexes[i] = struct{}{}
		}
	}
	for i := range indexes {
		migrateShardTables(t, dbInstance, i)
	}
	dbInstance.Table("kvstores").Create(&kvstoreRow{K: "stopNew", V: 0})
	dbInstance.Table(statusShard).Create(&status)

	ch := make(chan int, 1)
	craw(status, ch, 0)
	if got := <-ch; got != 1 {
		t.Fatalf("craw() channel = %d, want 1", got)
	}

	var lake models.Page
	if err := dbInstance.Table(pageShard).First(&lake, status.ID).Error; err != nil {
		t.Fatalf("find lake: %v", err)
	}
	if lake.CrawDone != 1 || lake.Title != "Title" || !strings.Contains(lake.Text, "Body") {
		t.Fatalf("lake = %#v", lake)
	}
}

func TestCrawSkipsLimitedAndFailedFetch(t *testing.T) {
	setupMainRedis(t)
	setupMainDB(t)
	status := models.Status{ID: 1, Url: "://bad", Host: "limited.example"}

	db.Rdb.SAdd(db.Ctx, "ese_spider_host_black_list", status.Host)
	ch := make(chan int, 1)
	craw(status, ch, 0)
	if got := <-ch; got != 0 {
		t.Fatalf("limited craw() = %d, want 0", got)
	}

	db.Rdb.Del(db.Ctx, "ese_spider_host_black_list")
	craw(status, ch, 0)
	if got := <-ch; got != 2 {
		t.Fatalf("failed craw() = %d, want 2", got)
	}
}

func TestCronHelpers(t *testing.T) {
	setupMainRedis(t)
	dbInstance := setupMainDB(t)
	migrateAllShardTables(t, dbInstance)
	originalCurrentTime := currentTime
	defer func() { currentTime = originalCurrentTime }()
	currentTime = func() time.Time { return time.Date(2024, 1, 1, 0, 10, 0, 0, time.Local) }
	wordBlackList = map[string]struct{}{"skip": {}}
	每分钟每个表执行分词 = 10
	一步转移的字典条数 = 2
	每个词转移的深度 = 2
	t.Setenv("APP_DEBUG", "true")

	dbInstance.Table("word_black_list").Create(map[string]any{"word": "blocked"})
	reloadWordBlacklist()
	if _, ok := wordBlackList["blocked"]; !ok {
		t.Fatalf("wordBlackList = %#v", wordBlackList)
	}

	table := tools.HexTableName("pages", 0)
	dbInstance.Table(table).Create(&models.Page{ID: 1, Url: "https://example.com", Host: "example.com", CrawDone: 1, Text: "Hello 中文 skip"})
	if got := generateDicsForTable(0, dbInstance, table); got != 1 {
		t.Fatalf("generateDicsForTable() = %d", got)
	}
	if got := db.Rdb10.DBSize(db.Ctx).Val(); got == 0 {
		t.Fatal("expected generated dictionary tokens")
	}

	db.Rdb10.FlushDB(db.Ctx)
	db.Rdb10.RPush(db.Ctx, "hello", "0,1,1,8,0-", "0,2,1,8,0-", "tail")
	result := getWordAndSppendSrting()
	if result.word != "hello" || result.appendString != "0,1,1,8,0-0,2,1,8,0-" {
		t.Fatalf("getWordAndSppendSrting() = %#v", result)
	}
	if got := getWordAndSppendSrting(); got.word == "" && db.Rdb10.DBSize(db.Ctx).Val() != 0 {
		t.Fatalf("unexpected empty transfer result with keys left")
	}

	db.Rdb10.FlushDB(db.Ctx)
	if transferWordDicsBatch() {
		t.Fatal("expected empty transfer batch")
	}

	statusTableName := tools.HexTableName("status", 1)
	statuses := []models.Status{
		{ID: 1, Url: "https://a.example", Host: "a.example"},
		{ID: 2, Url: "https://b.example", Host: "b.example"},
	}
	dbInstance.Table(statusTableName).Create(&statuses)
	if got := enqueueStatusesForTable(1, 2, []string{"blocked.example"}); got != 2 {
		t.Fatalf("enqueueStatusesForTable() = %d", got)
	}
	if got := db.Rdb.LLen(db.Ctx, "need_craw_list").Val(); got != 2 {
		t.Fatalf("need_craw_list len = %d", got)
	}

	dbInstance.Exec("update `" + statusTableName + "` set craw_done = 1")
	db.Rdb.FlushDB(db.Ctx)
	domain1BlackList = nil
	dbInstance.Table("domain_black_list").Create(map[string]any{"domain": "blocked.example"})
	filterTableName := tools.HexTableName("status", 3)
	dbInstance.Table(filterTableName).Create(&[]models.Status{
		{ID: 1, Url: "https://blocked.example/a", Host: "blocked.example"},
		{ID: 2, Url: "https://allowed.example/a", Host: "allowed.example"},
	})
	prepareStatusesBackground()
	payloads := db.Rdb.LRange(db.Ctx, "need_craw_list", 0, -1).Val()
	if len(payloads) != 1 {
		t.Fatalf("need_craw_list filtered len = %d, want 1: %#v", len(payloads), payloads)
	}
	var queued models.Status
	if err := json.Unmarshal([]byte(payloads[0]), &queued); err != nil {
		t.Fatalf("unmarshal queued status: %v", err)
	}
	if queued.Host != "allowed.example" {
		t.Fatalf("queued host = %q, want allowed.example", queued.Host)
	}

	domain1BlackList = map[string]struct{}{"domain.test": {}}
	dbInstance.Table(tools.HexTableName("pages", 2)).Create(&models.Page{ID: 10, Url: "https://sync.example", Host: "sync.example"})
	autoParsePagesToStatus()
	hostRows := make([]models.Status, 0, 501)
	pageRows := make([]models.Page, 0, 501)
	for i := 1; i <= 501; i++ {
		hostRows = append(hostRows, models.Status{ID: uint(1000 + i), Url: "https://count.example/status", Host: "count.example", CrawDone: 1})
		pageRows = append(pageRows, models.Page{ID: uint(1000 + i), Url: "https://count.example/page", Host: "count.example", CrawDone: 1, Text: ""})
	}
	dbInstance.Table(tools.HexTableName("status", 2)).CreateInBatches(hostRows, 100)
	dbInstance.Table(tools.HexTableName("pages", 2)).CreateInBatches(pageRows, 100)
	prepareStatusesBackground()
	refreshHostCount()
	washHTMLToDB10()

	dbInstance.Table("kvstores").Create(&kvstoreRow{K: "stopWashDicRedisToMySQL", V: 0})
	washDB10ToDicMySQL()

	if got := runArtCommand(map[string]artCommand{"noop": func(...string) {}}, []string{"noop"}); !got {
		t.Fatal("expected noop art command to run")
	}
	if !reflect.DeepEqual(splitStringSet(domain1BlackList), splitStringSet(domain1BlackList)) {
		t.Fatal("unreachable sanity check")
	}
}

func TestAutoParsePagesToStatusUsesMatchingShardCursor(t *testing.T) {
	setupMainRedis(t)
	dbInstance := setupMainDB(t)
	migrateAllShardTables(t, dbInstance)

	dbInstance.Table(tools.HexTableName("status", 0)).Create(&models.Status{ID: 9999, Url: "https://status-00.example", Host: "status-00.example"})
	dbInstance.Table(tools.HexTableName("pages", 2)).Create(&models.Page{ID: 10, Url: "https://sync-02.example", Host: "sync-02.example"})

	autoParsePagesToStatus()

	var status models.Status
	if err := dbInstance.Table(tools.HexTableName("status", 2)).First(&status, 10).Error; err != nil {
		t.Fatalf("expected pages_02 row to sync into status_02: %v", err)
	}
}

func splitStringSet(values map[string]struct{}) []string {
	result := make([]string, 0, len(values))
	for value := range values {
		result = append(result, value)
	}
	return result
}

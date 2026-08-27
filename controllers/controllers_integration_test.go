package controllers

import (
	"html/template"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/gin-gonic/gin"
	"github.com/go-redis/redis/v8"
	"github.com/johnlui/enterprise-search-engine/db"
	"github.com/johnlui/enterprise-search-engine/models"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupControllerDB(t *testing.T) *gorm.DB {
	t.Helper()

	dbInstance, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
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
	if err := dbInstance.Table("word_dics").AutoMigrate(&models.WordDic{}); err != nil {
		t.Fatalf("migrate word_dics: %v", err)
	}
	if err := dbInstance.AutoMigrate(&models.WordPosting{}); err != nil {
		t.Fatalf("migrate word_postings: %v", err)
	}
	if err := dbInstance.Table("pages_00").AutoMigrate(&models.Page{}); err != nil {
		t.Fatalf("migrate pages_00: %v", err)
	}
	if err := dbInstance.Table("pages_70").AutoMigrate(&models.Page{}); err != nil {
		t.Fatalf("migrate pages_70: %v", err)
	}
	if err := dbInstance.Table("status_70").AutoMigrate(&models.Status{}); err != nil {
		t.Fatalf("migrate status_70: %v", err)
	}
	return dbInstance
}

func setupControllerRedis(t *testing.T) *miniredis.Miniredis {
	t.Helper()

	server := miniredis.RunT(t)
	db.Rdb = redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { db.Rdb.Close() })
	return server
}

func newTestRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.SetHTMLTemplate(template.Must(template.New("templates").Parse(`
		{{define "search.tpl"}}{{.title}} {{.keyword}} {{.N}}{{range .values}} {{.Title}} {{.Brief}} {{.Url}}{{end}}{{end}}
		{{define "index.tpl"}}{{.title}}{{range .values}}{{range $k, $v := .}} {{$k}} {{$v}}{{end}}{{end}}{{end}}
	`)))
	router.GET("/", Search)
	router.GET("/status", SpiderStatus)
	return router
}

func TestEstimatedDocumentCountLocalAndCached(t *testing.T) {
	dbInstance := setupControllerDB(t)
	t.Setenv("APP_ENV", "local")
	if got := estimatedDocumentCount(time.Now()); got != 650000 {
		t.Fatalf("local estimatedDocumentCount() = %d", got)
	}

	t.Setenv("APP_ENV", "production")
	documentCountCache.mu.Lock()
	documentCountCache.value = 0
	documentCountCache.expiresAt = time.Time{}
	documentCountCache.mu.Unlock()

	dbInstance.Table("pages_70").Create(&models.Page{ID: 1, DicDone: 1})
	now := time.Now()
	if got := estimatedDocumentCount(now); got != 256 {
		t.Fatalf("estimatedDocumentCount() = %d, want 256", got)
	}
	dbInstance.Table("pages_70").Where("1 = 1").Delete(&models.Page{})
	if got := estimatedDocumentCount(now.Add(time.Second)); got != 256 {
		t.Fatalf("cached estimatedDocumentCount() = %d, want 256", got)
	}
}

func TestLoadWordDicsAndPagesByDocKey(t *testing.T) {
	dbInstance := setupControllerDB(t)

	if got := loadWordDics(nil); len(got) != 0 {
		t.Fatalf("loadWordDics(empty) = %#v", got)
	}
	dbInstance.Table("word_dics").Create(&models.WordDic{Name: "hello", Positions: "0,1,2,10,0-"})
	dics := loadWordDics([]string{"hello", "missing"})
	if dics["hello"].Positions == "" || len(dics) != 1 {
		t.Fatalf("loadWordDics() = %#v", dics)
	}

	dbInstance.Table("pages_00").Create(&models.Page{ID: 1, Title: "Hello", Url: "https://example.com", Text: "abc中文"})
	pages := loadPagesByDocKey([]string{"bad", "0-1", "x-2"})
	if pages["0-1"].Title != "Hello" || len(pages) != 1 {
		t.Fatalf("loadPagesByDocKey() = %#v", pages)
	}
}

func TestLoadDocPartsByWordPrefersStructuredPostings(t *testing.T) {
	dbInstance := setupControllerDB(t)

	dbInstance.Table("word_dics").Create(&models.WordDic{Name: "hello", Positions: "0,1,1,100,0-0,2,1,100,0-"})
	dbInstance.Table("word_postings").Create(&models.WordPosting{
		Term:          "hello",
		TableIndex:    0,
		DocID:         2,
		TermFrequency: 7,
		DocLength:     120,
		Positions:     "3",
	})

	partsByWord := loadDocPartsByWord([]string{"hello"})
	parts := partsByWord["hello"]
	partsByDocID := make(map[uint]docPart, len(parts))
	for _, part := range parts {
		partsByDocID[part.docID] = part
	}
	if len(partsByDocID) != 2 || partsByDocID[1].termFrequency != 1 || partsByDocID[2].termFrequency != 7 {
		t.Fatalf("parts = %#v", parts)
	}
}

func TestSearchHandler(t *testing.T) {
	dbInstance := setupControllerDB(t)
	t.Setenv("APP_ENV", "local")
	dbInstance.Table("word_dics").Create(&models.WordDic{Name: "hello", Positions: "0,1,3,100,0-0,1,1,100,1-"})
	dbInstance.Table("pages_00").Create(&models.Page{ID: 1, Title: "Hello Title", Url: "https://example.com", Text: "abc中文123"})

	router := newTestRouter()
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/?keyword=hello", nil)
	router.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d", response.Code)
	}
	body := response.Body.String()
	if !strings.Contains(body, "Hello Title") || !strings.Contains(body, "https://example.com") || !strings.Contains(body, "650000") {
		t.Fatalf("body = %q", body)
	}

	response = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodGet, "/", nil)
	router.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("empty keyword status = %d", response.Code)
	}
}

func TestSpiderStatusHandlerAndRedisGetter(t *testing.T) {
	dbInstance := setupControllerDB(t)
	setupControllerRedis(t)
	now := time.Now()
	dayKey := int(now.Unix()) / 86400

	db.Rdb.HSet(db.Ctx, "host_counts_crawd_"+strconv.Itoa(dayKey), "a", "10", "b", "bad")
	db.Rdb.HSet(db.Ctx, "host_counts_crawd_invalid_"+strconv.Itoa(dayKey), "a", "2")
	db.Rdb.Set(db.Ctx, redisMinuteKey("ese_spider_result_in_minute_", now, 60), "3", time.Hour)
	db.Rdb.Set(db.Ctx, redisMinuteKey("ese_spider_result_4_in_minute_", now, 60), "1", time.Hour)
	db.Rdb.Set(db.Ctx, redisMinuteKey("ese_spider_all_status_in_minute_", now, 60), "4", time.Hour)
	db.Rdb.Set(db.Ctx, redisMinuteKey("ese_spider_new_status_in_minute_", now, 60), "2", time.Hour)
	db.Rdb.LPush(db.Ctx, "need_craw_list", "job")
	dbInstance.Table("status_70").Create(&models.Status{ID: 1})

	if got := redisIntGetter()(redisMinuteKey("ese_spider_result_in_minute_", now, 60)); got != 3 {
		t.Fatalf("redisIntGetter() = %d", got)
	}

	router := newTestRouter()
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/status", nil)
	router.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d", response.Code)
	}
	body := response.Body.String()
	for _, want := range []string{"ESE状态监控面板", "待爬队列长度", "预估 URL 总数", "256", "已爬总数"} {
		if !strings.Contains(body, want) {
			t.Fatalf("body missing %q: %q", want, body)
		}
	}
}

func TestParseBestDocPartsAndTopDocKeysEdges(t *testing.T) {
	if got := parseBestDocParts(""); got != nil {
		t.Fatalf("parseBestDocParts(empty) = %#v", got)
	}
	parts := parseBestDocParts("bad-0,bad,1,2-0,1,bad,2-0,1,1,bad-0,2,1,2-")
	if len(parts) != 1 || parts[0].docKey != "0-2" {
		t.Fatalf("parseBestDocParts(edges) = %#v", parts)
	}
	if got := topDocKeysByScore(map[string]float64{"a": 1}, 0); got != nil {
		t.Fatalf("topDocKeysByScore(limit 0) = %#v", got)
	}
	if _, _, ok := parseDocKey("1-bad"); ok {
		t.Fatal("expected invalid doc id to fail")
	}
}

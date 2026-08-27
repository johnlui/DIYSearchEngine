package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/johnlui/enterprise-search-engine/config"
	"github.com/johnlui/enterprise-search-engine/controllers"
	"github.com/johnlui/enterprise-search-engine/db"
	"github.com/johnlui/enterprise-search-engine/internal/logging"
	"github.com/johnlui/enterprise-search-engine/internal/storage"
	"github.com/johnlui/enterprise-search-engine/models"
	"github.com/johnlui/enterprise-search-engine/tools"

	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
	"github.com/robfig/cron/v3"
	"gorm.io/gorm"
)

var domain1BlackList map[string]struct{}
var wordBlackList map[string]struct{}

var 一次爬取 = 4
var 爬取Worker数 = 64
var 一次准备 = 20

var 每分钟每个表执行分词 = 2
var 一步转移的字典条数 = 2000
var 每个词转移的深度 int64 = 10000

var activeConfig config.Config

var parseFlags = flag.Parse
var initializeENV = initENV
var initializeJieba = initJieba
var initializeDB = db.InitDB
var initializeArtCommands = initArtCommands
var launchServer = startServer
var createCron = func() *cron.Cron {
	return cron.New(
		cron.WithSeconds(),
		cron.WithChain(cron.SkipIfStillRunning(cron.DefaultLogger)),
	)
}
var startCron = func(c *cron.Cron) { c.Start() }
var runDictionaryWash = washDB10ToDicMySQL
var runSpider = nextStep
var blockMain = func() { select {} }
var artCommandFactory = artCommands
var debugDump = tools.DD
var runNextStepOnce = runNextStep
var loadAppConfig = config.Load
var runtimeRole = resolveRuntimeRole
var configureRuntimeServices = configureServices

func main() {
	// 处理启动参数
	parseFlags()
	role := runtimeRole(os.Args)

	// 加载 .env
	initializeENV()

	// 初始化结巴分词
	initializeJieba()

	// 初始化数据库
	initializeDB()
	configureRuntimeServices()

	// Art 命令行工具
	initializeArtCommands()

	runRole(role)
}

func runRole(role string) {
	switch role {
	case "serve":
		launchServer()
	case "crawler":
		runSpider(time.Now())
	case "scheduler":
		startScheduler()
		blockMain()
	case "indexer":
		runDictionaryWash()
	case "all":
		runAllRoles()
	default:
		fmt.Println("未知运行角色:", role)
		fmt.Println("可用角色: all, serve, crawler, scheduler, indexer")
	}
}

func runAllRoles() {
	// 启动 web 页面
	go launchServer()

	startScheduler()

	// 生产环境专用
	if !tools.ENV_DEBUG {
		go runDictionaryWash()
	}
	/*
	   spider
	*/
	// 开始爬
	runSpider(time.Now())

	// 阻塞，不跑爬虫时用于阻塞主线程
	blockMain()
}

func startScheduler() {
	c := createCron()
	registerSchedulerJobs(c)
	go startCron(c)
}

func registerSchedulerJobs(c *cron.Cron) {
	// 自动从 pages 复制数据到 status
	mustAddCronFunc(c, "*/20 * * * * *", autoParsePagesToStatus)
	// 将可以爬的 URL 插入 Redis
	mustAddCronFunc(c, "*/20 * * * * *", prepareStatusesBackground)
	// 五分钟刷新一次每个 host 的页面数量
	mustAddCronFunc(c, "0 */5 * * * *", refreshHostCount)
	// 分词，生成字典数据，并将数据插入 Redis
	mustAddCronFunc(c, "25 * * * * *", washHTMLToDB10)
	// 字典从 Redis 批量插入 MySQL
	mustAddCronFunc(c, "*/6 * * * * *", washDB10ToDicMySQL)
}

func mustAddCronFunc(c *cron.Cron, spec string, cmd func()) {
	if _, err := c.AddFunc(spec, cmd); err != nil {
		log.Fatalf("register cron %q: %v", spec, err)
	}
}

func initENV() {
	path, _ := os.Getwd()
	err := godotenv.Load(path + "/.env")
	fmt.Println("加载.env :", path+"/.env")
	if err != nil {
		log.Fatal("Error loading .env file")
	}
	activeConfig = loadAppConfig()
	applyRuntimeConfig(activeConfig)
	fmt.Println("APP_ENV:", activeConfig.AppEnv)
}

func applyRuntimeConfig(cfg config.Config) {
	tools.ENV_DEBUG = cfg.AppDebug
	一次爬取 = cfg.CrawlBatch
	爬取Worker数 = cfg.CrawlWorkers
	一次准备 = cfg.PrepareBatch
	每分钟每个表执行分词 = cfg.IndexPagesPerShard
	一步转移的字典条数 = cfg.IndexTransferBatch
	每个词转移的深度 = cfg.IndexTransferWordDepth
}

func configureServices() {
	tools.SetCurlFailureCounter(countCurlFailureWithRedis)
}

func countCurlFailureWithRedis(status models.Status) int {
	return storage.FromGlobals().CountCrawlFailure(status.Url)
}

func resolveRuntimeRole(args []string) string {
	if len(args) < 2 {
		return "all"
	}
	switch args[1] {
	case "all", "serve", "crawler", "scheduler", "indexer":
		return args[1]
	default:
		return "all"
	}
}
func initArtCommands() {
	argsWithProg := os.Args[1:]
	if len(argsWithProg) <= 1 || argsWithProg[0] != "art" {
		return
	}

	commands := artCommandFactory(Art{})
	if !runArtCommand(commands, argsWithProg[1:]) {
		debugDump("命令不存在")
	}

	debugDump("命令执行结束，退出")
}
func initJieba() {
	dictDir := path.Join(filepath.Dir(os.Args[0]), "dict")
	tools.InitJieba(dictDir)
}

// 循环爬
func nextStep(t time.Time) {
	for {
		startAt, shouldContinue := runNextStepOnce(t)
		t = startAt
		if !shouldContinue {
			return
		}
	}
}

// 真的爬，存储标题，内容，以及子链接
func craw(status models.Status, ch chan int, index int) {
	ch <- crawlStatus(status, index)
}

func crawlStatus(status models.Status, index int) int {
	now := time.Now()

	// 检查是否过于频繁
	if statusHostCrawIsTooMuch(status.Host) {
		// fmt.Println("过于频繁", time.Now().UnixMilli()-t.UnixMilli(), "毫秒")
		return 0
	}
	doc, chVal := tools.CurlContext(db.Ctx, status)

	// 如果失败，则不进行任何操作
	if chVal != 1 && chVal != 4 {
		// fmt.Println("curl失败", time.Now().UnixMilli()-t.UnixMilli(), "毫秒")
		return chVal
	}

	status.CrawTime = now
	title := tools.StringStrip(strings.TrimSpace(doc.Find("title").Text()))
	text := tools.StringStrip(strings.TrimSpace(doc.Text()))
	if err := storage.FromGlobals().SaveCrawledPage(status, title, text); err != nil {
		logging.Errorf("event=save_crawled_page url=%q error=%q", status.Url, err)
		return chVal
	}

	// 开始处理页面上新的超链接
	_stopNew, err := readKVInt("stopNew")
	if err != nil {
		logging.Errorf("event=read_control_flag flag=stopNew action=skip_discovery error=%q", err)
	} else if _stopNew == 1 {
		// fmt.Println("新URL全局开关关闭")
	} else {
		processDiscoveredLinks(status, collectDiscoveredLinks(doc), now)
	}

	// 写入 Redis，用于主动限流
	incrementHostCrawlWindows(status.Host, now)

	// fmt.Println("正常结束", time.Now().UnixMilli()-t.UnixMilli(), "毫秒")
	return chVal
}

func buildRouter() *gin.Engine {
	router := gin.Default()

	router.LoadHTMLGlob("views/*")

	// router.GET("/", _transStatus)
	router.GET("/", controllers.Search)
	router.GET("/status", controllers.SpiderStatus)
	return router
}

func startServer() {
	port := activeConfig.Port
	if port == "" {
		port = os.Getenv("PORT")
	}
	buildRouter().Run(":" + port)
}

func statusHostCrawIsTooMuch(host string) bool {
	return storage.FromGlobals().HostCrawlIsLimited(host, storageCrawlRateWindows(), time.Now())
}

func storageCrawlRateWindows() []storage.CrawlRateWindow {
	windows := make([]storage.CrawlRateWindow, 0, len(crawlRateWindows))
	for _, window := range crawlRateWindows {
		windows = append(windows, storage.CrawlRateWindow{
			Seconds: window.seconds,
			Limit:   window.limit,
		})
	}
	return windows
}

func realDB(url string) *gorm.DB {
	return storage.FromGlobals().DBForURL(url)
}

func statusTable(url string) func(tx *gorm.DB) *gorm.DB {
	return storage.FromGlobals().StatusScope(url)
}
func lakeTable(url string) func(tx *gorm.DB) *gorm.DB {
	return storage.FromGlobals().PageScope(url)
}
func md5Table(url string, table string) func(tx *gorm.DB) *gorm.DB {
	return storage.FromGlobals().TableScope(table, url)
}

func dd(v ...any) {
	debugDump(v...)
}

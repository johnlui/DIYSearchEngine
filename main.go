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

	"github.com/johnlui/enterprise-search-engine/controllers"
	"github.com/johnlui/enterprise-search-engine/db"
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
var 一次准备 = 20

var 每分钟每个表执行分词 = 2
var 一步转移的字典条数 = 2000
var 每个词转移的深度 int64 = 10000

func main() {
	// 处理启动参数
	flag.Parse()

	// 加载 .env
	initENV()

	// 初始化结巴分词
	initJieba()

	// 初始化数据库
	db.InitDB()

	// Art 命令行工具
	initArtCommands()

	// 启动 web 页面
	go startServer()

	// 定时任务
	c := cron.New(
		cron.WithSeconds(),
		cron.WithChain(cron.SkipIfStillRunning(cron.DefaultLogger)),
	)
	// 自动从 pages 复制数据到 status
	c.AddFunc("*/20 * * * * *", autoParsePagesToStatus)
	// 将可以爬的 URL 插入 Redis
	c.AddFunc("*/20 * * * * *", prepareStatusesBackground)
	// 五分钟刷新一次每个 host 的页面数量
	c.AddFunc("0 */5 * * * *", refreshHostCount)
	// 分词，生成字典数据，并将数据插入 Redis
	c.AddFunc("25 * * * * *", washHTMLToDB10)
	// 字典从 Redis 批量插入 MySQL
	c.AddFunc("*/6 * * * * *", washDB10ToDicMySQL)
	go c.Start()

	// 生产环境专用
	if !tools.ENV_DEBUG {
		washDB10ToDicMySQL()
	}
	/*
	   spider
	*/
	// 开始爬
	nextStep(time.Now())

	// 阻塞，不跑爬虫时用于阻塞主线程
	select {}
}

func initENV() {
	path, _ := os.Getwd()
	err := godotenv.Load(path + "/.env")
	fmt.Println("加载.env :", path+"/.env")
	if err != nil {
		log.Fatal("Error loading .env file")
	}
	tools.ENV_DEBUG = os.Getenv("APP_DEBUG") == "true"
	fmt.Println("APP_ENV:", os.Getenv("APP_ENV"))
}
func initArtCommands() {
	argsWithProg := os.Args[1:]
	if len(argsWithProg) <= 1 || argsWithProg[0] != "art" {
		return
	}

	commands := artCommands(Art{})
	if !runArtCommand(commands, argsWithProg[1:]) {
		tools.DD("命令不存在")
	}

	tools.DD("命令执行结束，退出")
}
func initJieba() {
	dictDir := path.Join(filepath.Dir(os.Args[0]), "dict")
	tools.InitJieba(dictDir)
}

// 循环爬
func nextStep(t time.Time) {
	for {
		startAt, shouldContinue := runNextStep(t)
		t = startAt
		if !shouldContinue {
			return
		}
	}
}

// 真的爬，存储标题，内容，以及子链接
func craw(status models.Status, ch chan int, index int) {
	now := time.Now()

	// 检查是否过于频繁
	if statusHostCrawIsTooMuch(status.Host) {
		ch <- 0
		// fmt.Println("过于频繁", time.Now().UnixMilli()-t.UnixMilli(), "毫秒")
		return
	}
	doc, chVal := tools.Curl(status)

	// 如果失败，则不进行任何操作
	if chVal != 1 && chVal != 4 {
		ch <- chVal

		// fmt.Println("curl失败", time.Now().UnixMilli()-t.UnixMilli(), "毫秒")
		return
	}

	// 更新 Status
	status.CrawDone = 1
	status.CrawTime = now
	realDB(status.Url).Scopes(statusTable(status.Url)).Save(&status)

	// 更新 Lake
	var lake models.Page
	realDB(status.Url).Scopes(lakeTable(status.Url)).Where(models.Page{ID: status.ID}).FirstOrCreate(&lake)

	lake.Url = status.Url
	lake.Host = status.Host
	lake.CrawDone = status.CrawDone
	lake.CrawTime = status.CrawTime
	lake.Title = tools.StringStrip(strings.TrimSpace(doc.Find("title").Text()))
	lake.Text = tools.StringStrip(strings.TrimSpace(doc.Text()))
	realDB(status.Url).Scopes(lakeTable(status.Url)).Save(&lake)

	// 开始处理页面上新的超链接
	_stopNew := -1
	db.DbInstance0.Table("kvstores").Where("k", "stopNew").Select("v").Find(&_stopNew)
	if _stopNew == -1 {
		fmt.Println("kvstores数据库连接失败，请检查 gorm-log.txt 日志")
		os.Exit(0)
	} else if _stopNew == 1 {
		// fmt.Println("新URL全局开关关闭")
	} else {
		processDiscoveredLinks(status, collectDiscoveredLinks(doc), now)
	}

	// 写入 Redis，用于主动限流
	incrementHostCrawlWindows(status.Host, now)

	ch <- chVal

	// fmt.Println("正常结束", time.Now().UnixMilli()-t.UnixMilli(), "毫秒")
}

func startServer() {

	router := gin.Default()

	router.LoadHTMLGlob("views/*")

	// router.GET("/", _transStatus)
	router.GET("/", controllers.Search)
	router.GET("/status", controllers.SpiderStatus)
	router.Run(":" + os.Getenv("PORT"))
}

func statusHostCrawIsTooMuch(host string) bool {
	hostBlackList, err := db.Rdb.SIsMember(db.Ctx, "ese_spider_host_black_list", host).Result()
	if err == nil && hostBlackList {
		return true
	}

	now := time.Now()
	pipe := db.Rdb.Pipeline()
	countCmds := make([]func() (int, error), len(crawlRateWindows))
	for i, window := range crawlRateWindows {
		cmd := pipe.Get(db.Ctx, tools.WindowBucketKey("ese_spider_xianliu_", host, window.seconds, now))
		countCmds[i] = cmd.Int
	}
	_, _ = pipe.Exec(db.Ctx)

	for i, getCount := range countCmds {
		count, err := getCount()
		if err == nil && count >= crawlRateWindows[i].limit {
			addHostToBlacklist(host)
			return true
		}
	}
	return false
}

func realDB(url string) *gorm.DB {
	// i, _ := strconv.ParseInt(tools.GetMD5Hash(url)[0:2], 16, 64)

	realDB := db.DbInstance0

	// 如果你有多个数据库，可以取消注释
	// if i > 127 {
	//   realDB = db.DbInstance1
	// }

	return realDB
}

func statusTable(url string) func(tx *gorm.DB) *gorm.DB {
	return md5Table(url, "status")
}
func lakeTable(url string) func(tx *gorm.DB) *gorm.DB {
	return md5Table(url, "pages")
}
func md5Table(url string, table string) func(tx *gorm.DB) *gorm.DB {
	return func(tx *gorm.DB) *gorm.DB {
		return tx.Table(tools.MD5TableName(table, url))
	}
}

func dd(v ...any) {
	tools.DD(v...)
}

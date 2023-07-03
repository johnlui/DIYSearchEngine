package main

import (
  "encoding/json"
  "flag"
  "fmt"
  "log"
  "net/url"
  "os"
  "path"
  "path/filepath"
  "reflect"
  "strconv"
  "strings"
  "time"

  "github.com/johnlui/enterprise-search-engine/controllers"
  "github.com/johnlui/enterprise-search-engine/db"
  "github.com/johnlui/enterprise-search-engine/models"
  "github.com/johnlui/enterprise-search-engine/tools"

  "github.com/PuerkitoBio/goquery"
  "github.com/gin-gonic/gin"
  "github.com/joho/godotenv"
  "github.com/robfig/cron/v3"
  "github.com/yanyiwu/gojieba"
  "golang.org/x/text/width"
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
  c := cron.New(cron.WithSeconds())
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

  if len(argsWithProg) > 1 && argsWithProg[0] == "art" {
    a := Art{}

    meth := reflect.ValueOf(a).MethodByName(tools.FirstLetterUppercase(strings.ToLower(argsWithProg[1])))

    if meth.IsValid() {
      if len(argsWithProg) > 2 {
        m := meth.Interface().(func(...string))
        m(argsWithProg[2:]...)
      } else {
        meth.Call([]reflect.Value{})
      }
    } else {
      tools.DD("命令不存在")
    }

    tools.DD("命令执行结束，退出")
  }
}
func initJieba() {
  dictDir := path.Join(filepath.Dir(os.Args[0]), "dict")
  jiebaPath := path.Join(dictDir, "jieba.dict.utf8")
  hmmPath := path.Join(dictDir, "hmm_model.utf8")
  userPath := path.Join(dictDir, "user.dict.utf8")
  idfPath := path.Join(dictDir, "idf.utf8")
  stopPath := path.Join(dictDir, "stop_words.utf8")
  tools.JiebaInstance = gojieba.NewJieba(jiebaPath, hmmPath, userPath, idfPath, stopPath)
}

// 循环爬
func nextStep(t time.Time) {
  // 判断爬虫开关是否关闭
  _stop := -1
  db.DbInstance0.Table("kvstores").Where("k", "stop").Select("v").Find(&_stop)
  if _stop == -1 {
    fmt.Println("kvstores数据库连接失败，请检查 gorm-log.txt 日志")
    os.Exit(0)
  } else if _stop == 1 {
    fmt.Println("全局开关关闭，30秒后再检测")
    time.Sleep(time.Second * 30)
    nextStep(time.Now())
  }

  // 重载一级域名黑名单
  domain1BlackList = map[string]struct{}{
    "huangye88.com": struct{}{},
    "gov.cn":        struct{}{},
  }
  _domain1BlackList := []string{}
  db.DbInstance0.Raw("select domain from domain_black_list").Scan(&_domain1BlackList)
  for _, v := range _domain1BlackList {
    domain1BlackList[v] = struct{}{}
  }

  // fmt.Println("开始准备本轮数据，计划共", maxNumber*256, "条")

  var statusArr []models.Status

  // statusCHs := make([]chan []models.Status, 256)

  maxNumber := 1
  if os.Getenv("APP_DEBUG") == "false" {
    maxNumber = 一次爬取
  }

  for i := 0; i < 256*maxNumber; i++ {
    jsonString := db.Rdb.RPop(db.Ctx, "need_craw_list").Val()
    var _status models.Status
    err := json.Unmarshal([]byte(jsonString), &_status)
    if err != nil {
      continue
    }
    statusArr = append(statusArr, _status)
  }

  validCount := len(statusArr)

  fmt.Println("本轮数据共", validCount, "条")

  if validCount == 0 {
    fmt.Println("本轮无数据，60秒后再检测")
    time.Sleep(time.Minute)
    nextStep(time.Now())
  }

  chs := make([]chan int, validCount)
  for k, v := range statusArr {
    chs[k] = make(chan int)
    go craw(v, chs[k], k)
  }

  var results = make(map[int]int)
  for _, ch := range chs {
    r := <-ch

    _, prs := results[r]
    if prs {
      results[r] += 1
    } else {
      results[r] = 1
    }
  }

  fmt.Println("跑完一轮", time.Now().Unix()-t.Unix(), "秒，有效",
    results[1], "条，略过",
    results[0], "条，网络错误",
    results[2], "条，多次网络错误置done",
    results[4], "条")
  if results[3] > 0 {
    fmt.Println("HTML解析失败", results[3], "条")
  }

  // 有效
  key := "ese_spider_result_in_minute_" + strconv.Itoa(int(time.Now().Unix())/60)
  db.Rdb.IncrBy(db.Ctx, key, int64(results[1])).Err()
  db.Rdb.Expire(db.Ctx, key, time.Hour).Err()
  // 多次网络错误置done
  key1 := "ese_spider_result_4_in_minute_" + strconv.Itoa(int(time.Now().Unix())/60)
  db.Rdb.IncrBy(db.Ctx, key1, int64(results[4])).Err()
  db.Rdb.Expire(db.Ctx, key1, time.Hour).Err()

  nextStep(time.Now())
}

// 真的爬，存储标题，内容，以及子链接
func craw(status models.Status, ch chan int, index int) {

  // 检查是否过于频繁
  if statusHostCrawIsTooMuch(status.Host) {
    ch <- 0
    // fmt.Println("过于频繁", time.Now().UnixMilli()-t.UnixMilli(), "毫秒")
    return
  }
  doc, chVal := tools.Curl(status, ch)

  // 如果失败，则不进行任何操作
  if chVal != 1 && chVal != 4 {
    ch <- chVal

    // fmt.Println("curl失败", time.Now().UnixMilli()-t.UnixMilli(), "毫秒")
    return
  }

  // 更新 Status
  status.CrawDone = 1
  status.CrawTime = time.Now()
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
    urlMap := make(map[string]int)
    doc.Find("a").Each(func(i int, s *goquery.Selection) {
      // For each item found, get the title
      title := strings.Trim(s.Text(), " \n")
      href := width.Narrow.String(strings.Trim(s.AttrOr("href", ""), " \n"))
      _url, _, _ := strings.Cut(href, "#")
      _url = strings.ToLower(_url)

      // 判断一个页面上是否有两个URL重复
      _, urlPrs := urlMap[_url]
      if urlPrs {
        return
      }
      urlMap[_url] = 1

      if tools.IsUrl(_url) {
        u, _ := url.Parse(_url)

        parts := strings.Split(u.Host, ".")
        domain1 := ""
        domain2 := ""
        if len(parts) >= 2 {
          domain1 = parts[len(parts)-2] + "." + parts[len(parts)-1]
          domain2 = domain1
          if len(parts) >= 3 {
            domain2 = parts[len(parts)-3] + "." + parts[len(parts)-2] + "." + parts[len(parts)-1]
          }
        }

        _, prs := domain1BlackList[domain1]
        if !prs {
          allStatusKey := "ese_spider_all_status_in_minute_" + strconv.Itoa(int(time.Now().Unix())/60)

          statusHashMapKey := "ese_spider_status_exist"
          statusExist := db.Rdb.HExists(db.Ctx, statusHashMapKey, _url).Val()
          // 若 HashMap 中不存在，则查询数据库
          if !statusExist {
            var newStatus models.Status
            result := realDB(_url).Scopes(statusTable(_url)).Where(models.Status{Url: _url}).FirstOrCreate(&newStatus)

            newStatus.Url = _url
            newStatus.Host = strings.ToLower(u.Host)
            newStatus.CrawTime, _ = time.Parse("2006-01-02 15:04:05", "2001-01-01 00:00:00")
            realDB(_url).Scopes(statusTable(_url)).Save(&newStatus)

            if result.RowsAffected > 0 {
              newStatusKey := "ese_spider_new_status_in_minute_" + strconv.Itoa(int(time.Now().Unix())/60)
              db.Rdb.IncrBy(db.Ctx, newStatusKey, 1).Err()
              db.Rdb.Expire(db.Ctx, newStatusKey, time.Hour).Err()
            }

            var newLake models.Page
            realDB(_url).Scopes(lakeTable(_url)).Where(models.Page{ID: newStatus.ID}).FirstOrCreate(&newLake)

            newLake.ID = newStatus.ID
            newLake.OriginTitle = title
            newLake.ReferrerId = status.ID
            newLake.Url = _url
            newLake.Scheme = strings.ToLower(u.Scheme)
            newLake.Host = strings.ToLower(u.Host)
            newLake.Domain1 = strings.ToLower(domain1)
            newLake.Domain2 = strings.ToLower(domain2)
            newLake.Path = u.Path
            newLake.Query = u.RawQuery
            newLake.CrawTime, _ = time.Parse("2006-01-02 15:04:05", "2001-01-01 00:00:00")
            realDB(_url).Scopes(lakeTable(_url)).Save(&newLake)

            // 无论是否新插入了数据，都将 _url 入 HashMap
            db.Rdb.HSet(db.Ctx, statusHashMapKey, _url, 1).Err()
          }

          db.Rdb.IncrBy(db.Ctx, allStatusKey, 1).Err()
          db.Rdb.Expire(db.Ctx, allStatusKey, time.Hour).Err()

          // fmt.Printf("新增写入 %s %s\n", title, _url)
        } else {
          // fmt.Printf("爬到旧的 %s %s\n", title, _url)
        }

      }
    })
  }

  // 写入 Redis，用于主动限流
  for _, t := range [][]int{
    []int{2, 1},
    []int{60, 15},
    []int{3600, 450},
    []int{86400, 5400},
  } {
    key := "ese_spider_xianliu_" + status.Host + "_" + strconv.Itoa(t[0]) + "s_" + strconv.FormatInt(time.Now().Unix()/int64(t[0]), 10)
    db.Rdb.IncrBy(db.Ctx, key, 1).Err()
    db.Rdb.Expire(db.Ctx, key, time.Second*time.Duration(t[0])).Err()
    // fmt.Println(key)
  }

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

  for _, t := range [][]int{
    []int{2, 1},
    []int{60, 15},
    []int{3600, 450},
    []int{86400, 5400},
  } {

    // host黑名单 redis 缓存
    hostBlackList, err := db.Rdb.SIsMember(db.Ctx, "ese_spider_host_black_list", host).Result()
    if err == nil && hostBlackList {
      return true
    }

    key := "ese_spider_xianliu_" + host + "_" + strconv.Itoa(t[0]) + "s_" + strconv.FormatInt(time.Now().Unix()/int64(t[0]), 10)

    count, err := db.Rdb.Get(db.Ctx, key).Int()
    if err == nil {
      if count >= t[1] {
        db.Rdb.SAdd(db.Ctx, "ese_spider_host_black_list", host)

        ese_spider_host_black_listTTL, _ := db.Rdb.TTL(db.Ctx, "ese_spider_host_black_list").Result()
        if ese_spider_host_black_listTTL == -1 {
          db.Rdb.Expire(db.Ctx, "ese_spider_host_black_list", time.Minute*42).Err()
        }
        // fmt.Println(strconv.Itoa(t[0])+"秒限制"+strconv.Itoa(t[1])+"条", host)
        return true
      }
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
    tableName := table + "_" + tools.GetMD5Hash(url)[0:2]
    return tx.Table(tableName)
  }
}

func dd(v ...any) {
  tools.DD(v)
}

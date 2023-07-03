package controllers

import (
  "sort"
  "strconv"
  "time"

  "github.com/gin-gonic/gin"
  "github.com/johnlui/enterprise-search-engine/db"
  "github.com/johnlui/enterprise-search-engine/tools"
)

func SpiderStatus(c *gin.Context) {
  // URL 总数
  keyAll := "host_counts_all_" + strconv.Itoa(int(time.Now().Unix())/86400)
  keyCrawd := "host_counts_crawd_" + strconv.Itoa(int(time.Now().Unix())/86400)
  keyCrawdInvalid := "host_counts_crawd_invalid_" + strconv.Itoa(int(time.Now().Unix())/86400)

  // 获得数量最多的10个域名
  type kv struct {
    Key   string
    Value int
  }
  var ss1 []kv
  rr1, _ := db.Rdb.HGetAll(db.Ctx, keyAll).Result()
  for k, v := range rr1 {
    _v, _ := strconv.Atoi(v)
    ss1 = append(ss1, kv{k, _v})
  }
  sort.Slice(ss1, func(i, j int) bool {
    return ss1[i].Value > ss1[j].Value
  })

  var ss []kv
  rr, _ := db.Rdb.HGetAll(db.Ctx, keyCrawd).Result()
  for k, v := range rr {
    _v, _ := strconv.Atoi(v)
    ss = append(ss, kv{k, _v})
  }
  sort.Slice(ss, func(i, j int) bool {
    return ss[i].Value > ss[j].Value
  })

  r1, _ := db.Rdb.HVals(db.Ctx, keyCrawd).Result()
  crawdCount := 0
  for _, v := range r1 {
    _count, _ := strconv.Atoi(v)
    crawdCount += _count
  }
  r2, _ := db.Rdb.HVals(db.Ctx, keyCrawdInvalid).Result()
  crawdCountInvalid := 0
  for _, v := range r2 {
    _count, _ := strconv.Atoi(v)
    crawdCountInvalid += _count
  }

  // 过去1分钟爬取
  lastMinuteCount, err := db.Rdb.Get(db.Ctx, "ese_spider_result_in_minute_"+strconv.Itoa(int(time.Now().Unix()-60)/60)).Result()
  if err != nil {
    lastMinuteCount = "0"
  }

  // 过去10分钟爬取
  last10MCount := 0
  for i := 0; i < 10; i++ {
    c, _ := db.Rdb.Get(db.Ctx, "ese_spider_result_in_minute_"+strconv.Itoa(int(time.Now().Unix())/60-i)).Int()
    last10MCount += c
  }

  // 过去1小时爬取
  lastHourCount := 0
  for i := 0; i < 60; i++ {
    c, _ := db.Rdb.Get(db.Ctx, "ese_spider_result_in_minute_"+strconv.Itoa(int(time.Now().Unix())/60-i)).Int()
    lastHourCount += c
  }

  // 过去1分钟爬取网络错误
  lastMinute4Count, err := db.Rdb.Get(db.Ctx, "ese_spider_result_4_in_minute_"+strconv.Itoa(int(time.Now().Unix()-60)/60)).Result()
  if err != nil {
    lastMinute4Count = "0"
  }

  // 过去10分钟爬取网络错误
  last10M4Count := 0
  for i := 0; i < 10; i++ {
    c, _ := db.Rdb.Get(db.Ctx, "ese_spider_result_4_in_minute_"+strconv.Itoa(int(time.Now().Unix())/60-i)).Int()
    last10M4Count += c
  }

  // 过去1小时爬取网络错误
  lastHour4Count := 0
  for i := 0; i < 60; i++ {
    c, _ := db.Rdb.Get(db.Ctx, "ese_spider_result_4_in_minute_"+strconv.Itoa(int(time.Now().Unix())/60-i)).Int()
    lastHour4Count += c
  }

  // 过去1分钟新增status 全部
  lastMinute4All, err := db.Rdb.Get(db.Ctx, "ese_spider_all_status_in_minute_"+strconv.Itoa(int(time.Now().Unix()-60)/60)).Result()
  if err != nil {
    lastMinute4All = "0"
  }

  // 过去10分钟新增status 全部
  last10M4All := 0
  for i := 0; i < 10; i++ {
    c, _ := db.Rdb.Get(db.Ctx, "ese_spider_all_status_in_minute_"+strconv.Itoa(int(time.Now().Unix())/60-i)).Int()
    last10M4All += c
  }

  // 过去1小时新增status 全部
  lastHour4All := 0
  for i := 0; i < 60; i++ {
    c, _ := db.Rdb.Get(db.Ctx, "ese_spider_all_status_in_minute_"+strconv.Itoa(int(time.Now().Unix())/60-i)).Int()
    lastHour4All += c
  }

  // 过去1分钟新增status 纯新增
  lastMinute4New, err := db.Rdb.Get(db.Ctx, "ese_spider_new_status_in_minute_"+strconv.Itoa(int(time.Now().Unix()-60)/60)).Result()
  if err != nil {
    lastMinute4New = "0"
  }

  // 过去10分钟新增status 纯新增
  last10M4New := 0
  for i := 0; i < 10; i++ {
    c, _ := db.Rdb.Get(db.Ctx, "ese_spider_new_status_in_minute_"+strconv.Itoa(int(time.Now().Unix())/60-i)).Int()
    last10M4New += c
  }

  // 过去1小时新增status 纯新增
  lastHour4New := 0
  for i := 0; i < 60; i++ {
    c, _ := db.Rdb.Get(db.Ctx, "ese_spider_new_status_in_minute_"+strconv.Itoa(int(time.Now().Unix())/60-i)).Int()
    lastHour4New += c
  }

  // 预估 URL 总数
  totalCount := 0
  db.DbInstance0.Raw("select count(*) from status_70").Scan(&totalCount)
  totalCount *= 256

  // 待爬队列长度
  need_craw_listLength := db.Rdb.LLen(db.Ctx, "need_craw_list").Val()

  values := []map[string]any{
    map[string]any{"待爬队列长度": need_craw_listLength},
    map[string]any{"预估 URL 总数": tools.AddDouhao(totalCount)},
    map[string]any{"已爬总数": tools.AddDouhao(crawdCount)},
    map[string]any{"已爬无效数": tools.AddDouhao(crawdCountInvalid)},
    map[string]any{"过去1分钟爬取 | 多次网络错误": lastMinuteCount + " | " + lastMinute4Count},
    map[string]any{"过去10分钟爬取 | 多次网络错误": strconv.Itoa(last10MCount) + " | " + strconv.Itoa(last10M4Count)},
    map[string]any{"过去1小时爬取 | 多次网络错误": strconv.Itoa(lastHourCount) + " | " + strconv.Itoa(lastHour4Count)},
    map[string]any{"过去1分钟新爬到status | 新页面": lastMinute4All + " | " + lastMinute4New},
    map[string]any{"过去10分钟新爬到status | 新页面": strconv.Itoa(last10M4All) + " | " + strconv.Itoa(last10M4New)},
    map[string]any{"过去1小时新爬到status | 新页面": strconv.Itoa(lastHour4All) + " | " + strconv.Itoa(lastHour4New)},
  }

  c.HTML(200, "index.tpl", gin.H{
    "title":  "ESE状态监控面板",
    "time":   time.Now().Format("2006-01-02 15:04:05"),
    "values": values,
  })
}

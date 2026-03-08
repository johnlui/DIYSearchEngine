package controllers

import (
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/johnlui/enterprise-search-engine/db"
	"github.com/johnlui/enterprise-search-engine/tools"
)

func SpiderStatus(c *gin.Context) {
	now := time.Now()

	// URL 总数
	keyCrawd := "host_counts_crawd_" + strconv.Itoa(int(now.Unix())/86400)
	keyCrawdInvalid := "host_counts_crawd_invalid_" + strconv.Itoa(int(now.Unix())/86400)

	r1, _ := db.Rdb.HVals(db.Ctx, keyCrawd).Result()
	crawdCount := sumStringInts(r1)
	r2, _ := db.Rdb.HVals(db.Ctx, keyCrawdInvalid).Result()
	crawdCountInvalid := sumStringInts(r2)

	// 过去1分钟爬取
	lastMinuteCount, err := db.Rdb.Get(db.Ctx, redisMinuteKey("ese_spider_result_in_minute_", now, 60)).Result()
	if err != nil {
		lastMinuteCount = "0"
	}

	// 过去10分钟爬取
	last10MCount := rollingMinuteSum(redisIntGetter(), "ese_spider_result_in_minute_", now, 10)

	// 过去1小时爬取
	lastHourCount := rollingMinuteSum(redisIntGetter(), "ese_spider_result_in_minute_", now, 60)

	// 过去1分钟爬取网络错误
	lastMinute4Count, err := db.Rdb.Get(db.Ctx, redisMinuteKey("ese_spider_result_4_in_minute_", now, 60)).Result()
	if err != nil {
		lastMinute4Count = "0"
	}

	// 过去10分钟爬取网络错误
	last10M4Count := rollingMinuteSum(redisIntGetter(), "ese_spider_result_4_in_minute_", now, 10)

	// 过去1小时爬取网络错误
	lastHour4Count := rollingMinuteSum(redisIntGetter(), "ese_spider_result_4_in_minute_", now, 60)

	// 过去1分钟新增status 全部
	lastMinute4All, err := db.Rdb.Get(db.Ctx, redisMinuteKey("ese_spider_all_status_in_minute_", now, 60)).Result()
	if err != nil {
		lastMinute4All = "0"
	}

	// 过去10分钟新增status 全部
	last10M4All := rollingMinuteSum(redisIntGetter(), "ese_spider_all_status_in_minute_", now, 10)

	// 过去1小时新增status 全部
	lastHour4All := rollingMinuteSum(redisIntGetter(), "ese_spider_all_status_in_minute_", now, 60)

	// 过去1分钟新增status 纯新增
	lastMinute4New, err := db.Rdb.Get(db.Ctx, redisMinuteKey("ese_spider_new_status_in_minute_", now, 60)).Result()
	if err != nil {
		lastMinute4New = "0"
	}

	// 过去10分钟新增status 纯新增
	last10M4New := rollingMinuteSum(redisIntGetter(), "ese_spider_new_status_in_minute_", now, 10)

	// 过去1小时新增status 纯新增
	lastHour4New := rollingMinuteSum(redisIntGetter(), "ese_spider_new_status_in_minute_", now, 60)

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
		"time":   now.Format("2006-01-02 15:04:05"),
		"values": values,
	})
}

func redisIntGetter() func(string) int {
	return func(key string) int {
		value, _ := db.Rdb.Get(db.Ctx, key).Int()
		return value
	}
}

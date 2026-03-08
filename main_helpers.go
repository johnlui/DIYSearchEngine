package main

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/johnlui/enterprise-search-engine/db"
	"github.com/johnlui/enterprise-search-engine/models"
	"github.com/johnlui/enterprise-search-engine/tools"
)

type artCommand func(...string)

func artCommands(a Art) map[string]artCommand {
	return map[string]artCommand{
		"init": func(_ ...string) {
			a.Init()
		},
	}
}

func runArtCommand(commands map[string]artCommand, args []string) bool {
	if len(args) == 0 {
		return false
	}

	command, ok := commands[args[0]]
	if !ok {
		return false
	}

	command(args[1:]...)
	return true
}

func runNextStep(startAt time.Time) (time.Time, bool) {
	// 判断爬虫开关是否关闭
	_stop := -1
	db.DbInstance0.Table("kvstores").Where("k", "stop").Select("v").Find(&_stop)
	if _stop == -1 {
		fmt.Println("kvstores数据库连接失败，请检查 gorm-log.txt 日志")
		os.Exit(0)
	}
	if _stop == 1 {
		fmt.Println("全局开关关闭，30秒后再检测")
		time.Sleep(time.Second * 30)
		return time.Now(), true
	}

	// 重载一级域名黑名单
	domain1BlackList = map[string]struct{}{
		"huangye88.com": {},
		"gov.cn":        {},
	}
	_domain1BlackList := []string{}
	db.DbInstance0.Raw("select domain from domain_black_list").Scan(&_domain1BlackList)
	for _, v := range _domain1BlackList {
		domain1BlackList[v] = struct{}{}
	}

	statusArr := loadStatusesForCrawling()
	validCount := len(statusArr)

	fmt.Println("本轮数据共", validCount, "条")
	if validCount == 0 {
		fmt.Println("本轮无数据，60秒后再检测")
		time.Sleep(time.Minute)
		return time.Now(), true
	}

	chs := make([]chan int, validCount)
	for k, v := range statusArr {
		chs[k] = make(chan int)
		go craw(v, chs[k], k)
	}

	results := collectCrawlResults(chs)
	fmt.Println("跑完一轮", time.Now().Unix()-startAt.Unix(), "秒，有效",
		results[1], "条，略过",
		results[0], "条，网络错误",
		results[2], "条，多次网络错误置done",
		results[4], "条")
	if results[3] > 0 {
		fmt.Println("HTML解析失败", results[3], "条")
	}

	key := tools.MinuteBucketKey("ese_spider_result_in_minute_", time.Now())
	db.Rdb.IncrBy(db.Ctx, key, int64(results[1])).Err()
	db.Rdb.Expire(db.Ctx, key, time.Hour).Err()

	key1 := tools.MinuteBucketKey("ese_spider_result_4_in_minute_", time.Now())
	db.Rdb.IncrBy(db.Ctx, key1, int64(results[4])).Err()
	db.Rdb.Expire(db.Ctx, key1, time.Hour).Err()

	return time.Now(), true
}

func loadStatusesForCrawling() []models.Status {
	statusArr := make([]models.Status, 0)

	maxNumber := 1
	if os.Getenv("APP_DEBUG") == "false" {
		maxNumber = 一次爬取
	}

	for i := 0; i < 256*maxNumber; i++ {
		jsonString := db.Rdb.RPop(db.Ctx, "need_craw_list").Val()
		var status models.Status
		if err := json.Unmarshal([]byte(jsonString), &status); err != nil {
			continue
		}
		statusArr = append(statusArr, status)
	}

	return statusArr
}

func collectCrawlResults(chs []chan int) map[int]int {
	results := make(map[int]int)
	for _, ch := range chs {
		results[<-ch]++
	}
	return results
}

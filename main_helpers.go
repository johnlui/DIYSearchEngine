package main

import (
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/johnlui/enterprise-search-engine/internal/logging"
	"github.com/johnlui/enterprise-search-engine/internal/storage"
	"github.com/johnlui/enterprise-search-engine/models"
)

type artCommand func(...string)

var sleep = time.Sleep

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
	_stop, err := readKVInt("stop")
	if err != nil {
		logging.Errorf("event=read_control_flag flag=stop action=retry error=%q", err)
		sleep(time.Second * 30)
		return time.Now(), true
	}
	if _stop == 1 {
		fmt.Println("全局开关关闭，30秒后再检测")
		sleep(time.Second * 30)
		return time.Now(), true
	}

	// 重载一级域名黑名单
	domain1BlackList = loadCurrentDomainBlacklist()

	statusArr := loadStatusesForCrawling()
	validCount := len(statusArr)

	fmt.Println("本轮数据共", validCount, "条")
	if validCount == 0 {
		fmt.Println("本轮无数据，60秒后再检测")
		sleep(time.Minute)
		return time.Now(), true
	}

	results := runCrawlBatch(statusArr)
	fmt.Println("跑完一轮", time.Now().Unix()-startAt.Unix(), "秒，有效",
		results[1], "条，略过",
		results[0], "条，网络错误",
		results[2], "条，多次网络错误置done",
		results[4], "条")
	if results[3] > 0 {
		fmt.Println("HTML解析失败", results[3], "条")
	}

	now := time.Now()
	storage.FromGlobals().RecordCrawlResults(results, now)

	return now, true
}

func runCrawlBatch(statusArr []models.Status) map[int]int {
	results := make(map[int]int)
	var mu sync.Mutex

	runWorkerPool(len(statusArr), 爬取Worker数, func(i int) int {
		code := crawlStatus(statusArr[i], i)
		mu.Lock()
		results[code]++
		mu.Unlock()
		return 0
	})

	return results
}

func loadStatusesForCrawling() []models.Status {
	maxNumber := 1
	if os.Getenv("APP_DEBUG") == "false" {
		maxNumber = 一次爬取
	}

	popCount := 256 * maxNumber
	return storage.FromGlobals().PopCrawlStatuses(popCount)
}

func collectCrawlResults(chs []chan int) map[int]int {
	results := make(map[int]int)
	for _, ch := range chs {
		results[<-ch]++
	}
	return results
}

func readKVInt(key string) (int, error) {
	return storage.FromGlobals().ReadKVInt(key)
}

func loadCurrentDomainBlacklist() map[string]struct{} {
	return storage.FromGlobals().LoadDomainBlacklist(defaultDomainBlacklist())
}

func defaultDomainBlacklist() map[string]struct{} {
	return map[string]struct{}{
		"huangye88.com": {},
		"gov.cn":        {},
	}
}

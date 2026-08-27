package controllers

import (
	"strconv"
	"time"

	"github.com/johnlui/enterprise-search-engine/internal/keys"
	"github.com/johnlui/enterprise-search-engine/internal/storage"
	"github.com/johnlui/enterprise-search-engine/tools"
)

type SpiderStatusService struct {
	Store storage.Handles
	Now   func() time.Time
}

func NewSpiderStatusService() SpiderStatusService {
	return SpiderStatusService{
		Store: storage.FromGlobals(),
		Now:   time.Now,
	}
}

func (s SpiderStatusService) Values() []map[string]any {
	now := s.Now()

	crawledCount := sumStringInts(s.Store.HostCountValues(keys.HostCountsCrawled, now))
	crawledInvalidCount := sumStringInts(s.Store.HostCountValues(keys.HostCountsInvalid, now))

	lastMinuteCount := s.Store.RedisString(keys.MinuteOffset(keys.SpiderResultPerMinute, now, 60), "0")
	last10MCount := rollingMinuteSum(s.Store.RedisInt, keys.SpiderResultPerMinute, now, 10)
	lastHourCount := rollingMinuteSum(s.Store.RedisInt, keys.SpiderResultPerMinute, now, 60)

	lastMinute4Count := s.Store.RedisString(keys.MinuteOffset(keys.SpiderRetiredPerMinute, now, 60), "0")
	last10M4Count := rollingMinuteSum(s.Store.RedisInt, keys.SpiderRetiredPerMinute, now, 10)
	lastHour4Count := rollingMinuteSum(s.Store.RedisInt, keys.SpiderRetiredPerMinute, now, 60)

	lastMinute4All := s.Store.RedisString(keys.MinuteOffset(keys.SpiderAllURLsPerMinute, now, 60), "0")
	last10M4All := rollingMinuteSum(s.Store.RedisInt, keys.SpiderAllURLsPerMinute, now, 10)
	lastHour4All := rollingMinuteSum(s.Store.RedisInt, keys.SpiderAllURLsPerMinute, now, 60)

	lastMinute4New := s.Store.RedisString(keys.MinuteOffset(keys.SpiderNewURLsPerMinute, now, 60), "0")
	last10M4New := rollingMinuteSum(s.Store.RedisInt, keys.SpiderNewURLsPerMinute, now, 10)
	lastHour4New := rollingMinuteSum(s.Store.RedisInt, keys.SpiderNewURLsPerMinute, now, 60)

	return []map[string]any{
		{"待爬队列长度": s.Store.CrawlQueueLength()},
		{"预估 URL 总数": tools.AddDouhao(s.Store.EstimatedStatusCount())},
		{"已爬总数": tools.AddDouhao(crawledCount)},
		{"已爬无效数": tools.AddDouhao(crawledInvalidCount)},
		{"过去1分钟爬取 | 多次网络错误": lastMinuteCount + " | " + lastMinute4Count},
		{"过去10分钟爬取 | 多次网络错误": strconv.Itoa(last10MCount) + " | " + strconv.Itoa(last10M4Count)},
		{"过去1小时爬取 | 多次网络错误": strconv.Itoa(lastHourCount) + " | " + strconv.Itoa(lastHour4Count)},
		{"过去1分钟新爬到status | 新页面": lastMinute4All + " | " + lastMinute4New},
		{"过去10分钟新爬到status | 新页面": strconv.Itoa(last10M4All) + " | " + strconv.Itoa(last10M4New)},
		{"过去1小时新爬到status | 新页面": strconv.Itoa(lastHour4All) + " | " + strconv.Itoa(lastHour4New)},
	}
}

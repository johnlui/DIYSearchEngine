package keys

import (
	"strconv"
	"time"

	"github.com/johnlui/enterprise-search-engine/tools"
)

const (
	CrawlQueue              = "need_craw_list"
	SpiderHostBlacklist     = "ese_spider_host_black_list"
	SpiderKnownStatuses     = "ese_spider_status_exist"
	TransferredWords        = "HasBeenTransported"
	SpiderResultPerMinute   = "ese_spider_result_in_minute_"
	SpiderRetiredPerMinute  = "ese_spider_result_4_in_minute_"
	SpiderAllURLsPerMinute  = "ese_spider_all_status_in_minute_"
	SpiderNewURLsPerMinute  = "ese_spider_new_status_in_minute_"
	HostCountsAll           = "host_counts_all_"
	HostCountsCrawled       = "host_counts_crawd_"
	HostCountsInvalid       = "host_counts_crawd_invalid_"
	StatusQueueCursorPrefix = "table_"
	StatusQueueCursorSuffix = "_max_into_queue_id"
	CrawlFailureCountPrefix = "ese_spider_wangluocuowu_"
	CrawlRateLimitPrefix    = "ese_spider_xianliu_"
)

func Minute(prefix string, now time.Time) string {
	return tools.MinuteBucketKey(prefix, now)
}

func MinuteOffset(prefix string, now time.Time, minuteOffset int64) string {
	return prefix + strconv.FormatInt((now.Unix()-minuteOffset)/60, 10)
}

func Day(prefix string, now time.Time) string {
	return prefix + strconv.Itoa(int(now.Unix())/86400)
}

func CrawlFailureCount(url string) string {
	return CrawlFailureCountPrefix + tools.GetMD5Hash(url)
}

func CrawlRateLimit(host string, windowSeconds int, now time.Time) string {
	return tools.WindowBucketKey(CrawlRateLimitPrefix, host, windowSeconds, now)
}

func StatusQueueCursor(tableName string) string {
	return StatusQueueCursorPrefix + tableName + StatusQueueCursorSuffix
}

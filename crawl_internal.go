package main

import (
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/go-redis/redis/v8"
	"github.com/johnlui/enterprise-search-engine/db"
	"github.com/johnlui/enterprise-search-engine/models"
	"github.com/johnlui/enterprise-search-engine/tools"
	"golang.org/x/text/width"
)

type crawlRateWindow struct {
	seconds int
	limit   int
}

type discoveredLink struct {
	title   string
	url     string
	scheme  string
	host    string
	domain1 string
	domain2 string
	path    string
	query   string
}

var crawlRateWindows = []crawlRateWindow{
	{seconds: 2, limit: 1},
	{seconds: 60, limit: 15},
	{seconds: 3600, limit: 450},
	{seconds: 86400, limit: 5400},
}

var pendingCrawTime, _ = time.ParseInLocation("2006-01-02 15:04:05", "2001-01-01 00:00:00", time.Local)

func collectDiscoveredLinks(doc *goquery.Document) []discoveredLink {
	urlMap := make(map[string]struct{})
	links := make([]discoveredLink, 0)

	doc.Find("a").Each(func(i int, s *goquery.Selection) {
		title := strings.Trim(s.Text(), " \n")
		href := width.Narrow.String(strings.Trim(s.AttrOr("href", ""), " \n"))
		normalizedURL, _, _ := strings.Cut(href, "#")
		normalizedURL = strings.ToLower(normalizedURL)

		if normalizedURL == "" {
			return
		}
		if _, ok := urlMap[normalizedURL]; ok {
			return
		}
		urlMap[normalizedURL] = struct{}{}

		if !tools.IsUrl(normalizedURL) {
			return
		}

		parsedURL, err := url.Parse(normalizedURL)
		if err != nil {
			return
		}

		host := strings.ToLower(parsedURL.Host)
		domain1, domain2 := splitDomains(host)
		if _, blocked := domain1BlackList[domain1]; blocked {
			return
		}

		links = append(links, discoveredLink{
			title:   title,
			url:     normalizedURL,
			scheme:  strings.ToLower(parsedURL.Scheme),
			host:    host,
			domain1: strings.ToLower(domain1),
			domain2: strings.ToLower(domain2),
			path:    parsedURL.Path,
			query:   parsedURL.RawQuery,
		})
	})

	return links
}

func processDiscoveredLinks(status models.Status, links []discoveredLink, now time.Time) {
	if len(links) == 0 {
		return
	}

	const statusHashMapKey = "ese_spider_status_exist"
	statusExists := statusExistenceMap(statusHashMapKey, links)
	urlsToCache := make([]string, 0, len(links))
	newStatusCount := 0

	for _, link := range links {
		if statusExists[link.url] {
			continue
		}

		var newStatus models.Status
		result := realDB(link.url).Scopes(statusTable(link.url)).Where(models.Status{Url: link.url}).FirstOrCreate(&newStatus)

		newStatus.Url = link.url
		newStatus.Host = link.host
		newStatus.CrawTime = pendingCrawTime
		realDB(link.url).Scopes(statusTable(link.url)).Save(&newStatus)

		if result.RowsAffected > 0 {
			newStatusCount++
		}

		var newLake models.Page
		realDB(link.url).Scopes(lakeTable(link.url)).Where(models.Page{ID: newStatus.ID}).FirstOrCreate(&newLake)

		newLake.ID = newStatus.ID
		newLake.OriginTitle = link.title
		newLake.ReferrerId = status.ID
		newLake.Url = link.url
		newLake.Scheme = link.scheme
		newLake.Host = link.host
		newLake.Domain1 = link.domain1
		newLake.Domain2 = link.domain2
		newLake.Path = link.path
		newLake.Query = link.query
		newLake.CrawTime = pendingCrawTime
		realDB(link.url).Scopes(lakeTable(link.url)).Save(&newLake)

		urlsToCache = append(urlsToCache, link.url)
	}

	cacheKnownStatuses(statusHashMapKey, urlsToCache)
	incrementDiscoveredStatusCounters(len(links), newStatusCount, now)
}

func splitDomains(host string) (string, string) {
	parts := strings.Split(host, ".")
	if len(parts) < 2 {
		return "", ""
	}

	domain1 := parts[len(parts)-2] + "." + parts[len(parts)-1]
	domain2 := domain1
	if len(parts) >= 3 {
		domain2 = parts[len(parts)-3] + "." + parts[len(parts)-2] + "." + parts[len(parts)-1]
	}

	return domain1, domain2
}

func statusExistenceMap(hashKey string, links []discoveredLink) map[string]bool {
	if len(links) == 0 {
		return map[string]bool{}
	}

	pipe := db.Rdb.Pipeline()
	cmds := make([]*redis.BoolCmd, len(links))
	for i, link := range links {
		cmds[i] = pipe.HExists(db.Ctx, hashKey, link.url)
	}
	_, _ = pipe.Exec(db.Ctx)

	result := make(map[string]bool, len(links))
	for i, cmd := range cmds {
		exists, err := cmd.Result()
		if err != nil {
			continue
		}
		result[links[i].url] = exists
	}

	return result
}

func cacheKnownStatuses(hashKey string, urls []string) {
	if len(urls) == 0 {
		return
	}

	values := make([]any, 0, len(urls)*2)
	for _, url := range urls {
		values = append(values, url, 1)
	}
	db.Rdb.HSet(db.Ctx, hashKey, values...).Err()
}

func incrementDiscoveredStatusCounters(allCount, newCount int, now time.Time) {
	pipe := db.Rdb.Pipeline()
	if allCount > 0 {
		key := tools.MinuteBucketKey("ese_spider_all_status_in_minute_", now)
		pipe.IncrBy(db.Ctx, key, int64(allCount))
		pipe.Expire(db.Ctx, key, time.Hour)
	}
	if newCount > 0 {
		key := tools.MinuteBucketKey("ese_spider_new_status_in_minute_", now)
		pipe.IncrBy(db.Ctx, key, int64(newCount))
		pipe.Expire(db.Ctx, key, time.Hour)
	}
	_, _ = pipe.Exec(db.Ctx)
}

func incrementHostCrawlWindows(host string, now time.Time) {
	pipe := db.Rdb.Pipeline()
	for _, window := range crawlRateWindows {
		key := tools.WindowBucketKey("ese_spider_xianliu_", host, window.seconds, now)
		pipe.IncrBy(db.Ctx, key, 1)
		pipe.Expire(db.Ctx, key, time.Second*time.Duration(window.seconds))
	}
	_, _ = pipe.Exec(db.Ctx)
}

func addHostToBlacklist(host string) {
	db.Rdb.SAdd(db.Ctx, "ese_spider_host_black_list", host)

	ttl, _ := db.Rdb.TTL(db.Ctx, "ese_spider_host_black_list").Result()
	if ttl == -1 {
		db.Rdb.Expire(db.Ctx, "ese_spider_host_black_list", time.Minute*42).Err()
	}
}

func runWorkerPool(jobCount, workerCount int, fn func(int) int) int {
	if jobCount <= 0 {
		return 0
	}
	if workerCount <= 0 || workerCount > jobCount {
		workerCount = jobCount
	}

	jobs := make(chan int, jobCount)
	results := make(chan int, jobCount)
	var wg sync.WaitGroup

	for i := 0; i < workerCount; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for job := range jobs {
				results <- fn(job)
			}
		}()
	}

	for job := 0; job < jobCount; job++ {
		jobs <- job
	}
	close(jobs)

	wg.Wait()
	close(results)

	total := 0
	for result := range results {
		total += result
	}

	return total
}

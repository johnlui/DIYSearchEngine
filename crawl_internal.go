package main

import (
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/johnlui/enterprise-search-engine/internal/keys"
	"github.com/johnlui/enterprise-search-engine/internal/storage"
	"github.com/johnlui/enterprise-search-engine/models"
	"github.com/johnlui/enterprise-search-engine/tools"
	"golang.org/x/text/width"
)

type crawlRateWindow struct {
	seconds int
	limit   int
}

type discoveredLink = storage.DiscoveredLink

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
			Title:   title,
			URL:     normalizedURL,
			Scheme:  strings.ToLower(parsedURL.Scheme),
			Host:    host,
			Domain1: strings.ToLower(domain1),
			Domain2: strings.ToLower(domain2),
			Path:    parsedURL.Path,
			Query:   parsedURL.RawQuery,
		})
	})

	return links
}

func processDiscoveredLinks(status models.Status, links []discoveredLink, now time.Time) {
	if len(links) == 0 {
		return
	}

	store := storage.FromGlobals()
	statusExists := statusExistenceMap(keys.SpiderKnownStatuses, links)
	urlsToCache := make([]string, 0, len(links))
	newStatusCount := 0

	for _, link := range links {
		if statusExists[link.URL] {
			continue
		}

		created, err := store.SaveDiscoveredLink(status, link, pendingCrawTime)
		if err != nil {
			continue
		}
		if created {
			newStatusCount++
		}
		urlsToCache = append(urlsToCache, link.URL)
	}

	cacheKnownStatuses(keys.SpiderKnownStatuses, urlsToCache)
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
	return storage.FromGlobals().StatusExistenceMap(hashKey, links)
}

func cacheKnownStatuses(hashKey string, urls []string) {
	storage.FromGlobals().CacheKnownStatuses(hashKey, urls)
}

func incrementDiscoveredStatusCounters(allCount, newCount int, now time.Time) {
	storage.FromGlobals().IncrementDiscoveredStatusCounters(allCount, newCount, now)
}

func incrementHostCrawlWindows(host string, now time.Time) {
	storage.FromGlobals().IncrementHostCrawlWindows(host, storageCrawlRateWindows(), now)
}

func addHostToBlacklist(host string) {
	storage.FromGlobals().AddHostToBlacklist(host)
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

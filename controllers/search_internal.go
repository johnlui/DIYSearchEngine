package controllers

import (
	"container/heap"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/johnlui/enterprise-search-engine/db"
	"github.com/johnlui/enterprise-search-engine/models"
	"github.com/johnlui/enterprise-search-engine/tools"
)

const documentCountCacheTTL = time.Minute

type docPart struct {
	docKey        string
	tableIndex    int
	docID         uint
	termFrequency int
	docLength     int
}

type scoreEntry struct {
	key   string
	score float64
}

type scoreMinHeap []scoreEntry

func (h scoreMinHeap) Len() int           { return len(h) }
func (h scoreMinHeap) Less(i, j int) bool { return h[i].score < h[j].score }
func (h scoreMinHeap) Swap(i, j int)      { h[i], h[j] = h[j], h[i] }

func (h *scoreMinHeap) Push(x any) {
	*h = append(*h, x.(scoreEntry))
}

func (h *scoreMinHeap) Pop() any {
	old := *h
	n := len(old)
	item := old[n-1]
	*h = old[:n-1]
	return item
}

var documentCountCache struct {
	mu        sync.RWMutex
	value     int
	expiresAt time.Time
}

func estimatedDocumentCount(now time.Time) int {
	if os.Getenv("APP_ENV") == "local" {
		return 650000
	}

	documentCountCache.mu.RLock()
	if now.Before(documentCountCache.expiresAt) {
		value := documentCountCache.value
		documentCountCache.mu.RUnlock()
		return value
	}
	documentCountCache.mu.RUnlock()

	var count int
	db.DbInstance0.Raw("select count(*) from pages_70 where dic_done = 1").Scan(&count)
	count *= 256

	documentCountCache.mu.Lock()
	documentCountCache.value = count
	documentCountCache.expiresAt = now.Add(documentCountCacheTTL)
	documentCountCache.mu.Unlock()

	return count
}

func loadWordDics(words []string) map[string]models.WordDic {
	if len(words) == 0 {
		return map[string]models.WordDic{}
	}

	dics := make([]models.WordDic, 0, len(words))
	db.DbInstanceDic.Where("name IN ?", words).Find(&dics)

	result := make(map[string]models.WordDic, len(dics))
	for _, dic := range dics {
		result[dic.Name] = dic
	}

	return result
}

func uniqueStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}

func parseBestDocParts(positions string) []docPart {
	if positions == "" {
		return nil
	}

	bestParts := make(map[string]docPart)
	for _, rawPart := range strings.Split(positions, "-") {
		if rawPart == "" {
			continue
		}

		ints := strings.Split(rawPart, ",")
		if len(ints) < 4 {
			continue
		}

		tableIndex, err := strconv.Atoi(ints[0])
		if err != nil {
			continue
		}

		docID, err := strconv.ParseUint(ints[1], 10, 64)
		if err != nil {
			continue
		}

		termFrequency, err := strconv.Atoi(ints[2])
		if err != nil {
			continue
		}

		docLength, err := strconv.Atoi(ints[3])
		if err != nil {
			continue
		}

		docKey := ints[0] + "-" + ints[1]
		part := docPart{
			docKey:        docKey,
			tableIndex:    tableIndex,
			docID:         uint(docID),
			termFrequency: termFrequency,
			docLength:     docLength,
		}

		if current, ok := bestParts[docKey]; !ok || part.termFrequency > current.termFrequency {
			bestParts[docKey] = part
		}
	}

	result := make([]docPart, 0, len(bestParts))
	for _, part := range bestParts {
		result = append(result, part)
	}

	return result
}

func topDocKeysByScore(docsScores map[string]float64, limit int) []string {
	if limit <= 0 || len(docsScores) == 0 {
		return nil
	}

	if len(docsScores) <= limit {
		return sortDocKeysByScore(docsScores)
	}

	h := make(scoreMinHeap, 0, limit)
	for key, score := range docsScores {
		if len(h) < limit {
			heap.Push(&h, scoreEntry{key: key, score: score})
			continue
		}

		if score <= h[0].score {
			continue
		}

		heap.Pop(&h)
		heap.Push(&h, scoreEntry{key: key, score: score})
	}

	result := make([]string, len(h))
	for i := len(result) - 1; i >= 0; i-- {
		result[i] = heap.Pop(&h).(scoreEntry).key
	}

	return result
}

func loadPagesByDocKey(keys []string) map[string]models.Page {
	idsByTable := make(map[int][]uint)
	for _, key := range keys {
		tableIndex, docID, ok := parseDocKey(key)
		if !ok {
			continue
		}
		idsByTable[tableIndex] = append(idsByTable[tableIndex], docID)
	}

	pagesByDocKey := make(map[string]models.Page, len(keys))
	for tableIndex, ids := range idsByTable {
		tableName := tools.HexTableName("pages", tableIndex)
		var pages []models.Page
		db.DbInstance0.Table(tableName).Where("id IN ?", ids).Find(&pages)

		for _, page := range pages {
			docKey := fmt.Sprintf("%d-%d", tableIndex, page.ID)
			pagesByDocKey[docKey] = page
		}
	}

	return pagesByDocKey
}

func parseDocKey(value string) (int, uint, bool) {
	tableRaw, idRaw, ok := strings.Cut(value, "-")
	if !ok {
		return 0, 0, false
	}

	tableIndex, err := strconv.Atoi(tableRaw)
	if err != nil {
		return 0, 0, false
	}

	docID, err := strconv.ParseUint(idRaw, 10, 64)
	if err != nil {
		return 0, 0, false
	}

	return tableIndex, uint(docID), true
}

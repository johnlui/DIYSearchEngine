package controllers

import (
	"container/heap"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/johnlui/enterprise-search-engine/internal/storage"
	"github.com/johnlui/enterprise-search-engine/models"
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

	count := storage.FromGlobals().EstimatedIndexedDocumentCount()

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

	return storage.FromGlobals().LoadWordDics(words)
}

func loadDocPartsByWord(words []string) map[string][]docPart {
	result := make(map[string][]docPart, len(words))
	if len(words) == 0 {
		return result
	}

	postingsByWord := storage.FromGlobals().LoadPostings(words)
	for word, postings := range postingsByWord {
		result[word] = docPartsFromPostings(postings)
	}

	for word, dic := range loadWordDics(words) {
		result[word] = mergeDocParts(result[word], parseBestDocParts(dic.Positions))
	}

	return result
}

func docPartsFromPostings(postings []models.WordPosting) []docPart {
	parts := make([]docPart, 0, len(postings))
	for _, posting := range postings {
		docKey := strconv.Itoa(posting.TableIndex) + "-" + strconv.Itoa(int(posting.DocID))
		parts = append(parts, docPart{
			docKey:        docKey,
			tableIndex:    posting.TableIndex,
			docID:         posting.DocID,
			termFrequency: posting.TermFrequency,
			docLength:     posting.DocLength,
		})
	}
	return parts
}

func mergeDocParts(primary, fallback []docPart) []docPart {
	if len(primary) == 0 {
		return fallback
	}
	if len(fallback) == 0 {
		return primary
	}

	seen := make(map[string]struct{}, len(primary))
	merged := make([]docPart, 0, len(primary)+len(fallback))
	for _, part := range primary {
		seen[part.docKey] = struct{}{}
		merged = append(merged, part)
	}

	for _, part := range fallback {
		if _, ok := seen[part.docKey]; ok {
			continue
		}
		seen[part.docKey] = struct{}{}
		merged = append(merged, part)
	}

	return merged
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

	return storage.FromGlobals().LoadPagesByTableIDs(idsByTable)
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

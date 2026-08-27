package controllers

import (
	"errors"
	"math"
	"time"
	"unicode/utf8"

	"github.com/johnlui/enterprise-search-engine/models"
	"github.com/johnlui/enterprise-search-engine/tools"
)

var errZeroDocuments = errors.New("文档总数N不能为零")

type SearchResponse struct {
	N      int
	Values []SearchResult
}

type SearchService struct {
	Tokenizer              func(string) []string
	LoadDocPartsByWord     func([]string) map[string][]docPart
	EstimatedDocumentCount func(time.Time) int
	LoadPagesByDocKey      func([]string) map[string]models.Page
	Now                    func() time.Time
}

func NewSearchService() SearchService {
	return SearchService{
		Tokenizer:              tools.GetFenciResultArray,
		LoadDocPartsByWord:     loadDocPartsByWord,
		EstimatedDocumentCount: estimatedDocumentCount,
		LoadPagesByDocKey:      loadPagesByDocKey,
		Now:                    time.Now,
	}
}

func (s SearchService) Search(keyword string) (SearchResponse, error) {
	response := SearchResponse{
		Values: make([]SearchResult, 0),
	}

	if utf8.RuneCountInString(keyword) == 0 {
		return response, nil
	}

	words := uniqueStrings(s.Tokenizer(keyword))
	docsScores := make(map[string]float64)
	docPartsByWord := s.LoadDocPartsByWord(words)

	response.N = s.EstimatedDocumentCount(s.Now())
	if response.N == 0 {
		return response, errZeroDocuments
	}

	for _, word := range words {
		partsArr := docPartsByWord[word]
		if len(partsArr) == 0 {
			continue
		}

		nqi := len(partsArr)
		idf := math.Log10((float64(response.N-nqi) + 0.5) / (float64(nqi) + 0.5))

		for _, part := range partsArr {
			docsScores[part.docKey] += idf * bm25TermScore(part)
		}
	}

	keys := topDocKeysByScore(docsScores, 200)
	pagesByDocKey := s.LoadPagesByDocKey(keys)

	for _, doc := range keys {
		lake, ok := pagesByDocKey[doc]
		if !ok {
			continue
		}

		response.Values = append(response.Values, SearchResult{
			Title: lake.Title,
			Score: docsScores[doc],
			Brief: briefForSearchResult(lake.Text),
			Url:   lake.Url,
		})
	}

	return response, nil
}

func bm25TermScore(part docPart) float64 {
	k1 := 2.0
	b := 0.75
	avgDocLength := 13214.0

	return (float64(part.termFrequency) * (k1 + 1)) /
		(float64(part.termFrequency) + k1*(1-b+b*(float64(part.docLength)/avgDocLength)))
}

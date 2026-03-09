package controllers

import (
	"math"
	"time"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
	"github.com/johnlui/enterprise-search-engine/tools"
)

type SearchResult struct {
	Title string
	Score float64
	Brief string
	Url   string
}

func Search(c *gin.Context) {
	t := time.Now()

	keyword := c.Query("keyword")

	N := 0
	values := make([]SearchResult, 0)

	if utf8.RuneCountInString(keyword) > 0 {
		r := uniqueStrings(tools.GetFenciResultArray(keyword))
		docsScores := make(map[string]float64)
		wordDics := loadWordDics(r)

		N = estimatedDocumentCount(time.Now())
		if N == 0 {
			panic("文档总数N不能为零")
		}

		for _, v := range r {
			dic, ok := wordDics[v]
			if !ok {
				continue
			}

			partsArr := parseBestDocParts(dic.Positions)
			if len(partsArr) == 0 {
				continue
			}

			NQi := len(partsArr)
			IDF := math.Log10((float64(N-NQi) + 0.5) / (float64(NQi) + 0.5))

			for _, p := range partsArr {
				// https://zhuanlan.zhihu.com/p/499906089

				k1 := 2.0
				b := 0.75
				// 平均文档长度，暂时没用，没有记录文档长度
				avgDocLength := 13214.0

				RQiDj := (float64(p.termFrequency) * (k1 + 1)) / (float64(p.termFrequency) + k1*(1-b+b*(float64(p.docLength)/avgDocLength)))

				docsScores[p.docKey] += IDF * RQiDj
			}
		}

		keys := topDocKeysByScore(docsScores, 200)
		pagesByDocKey := loadPagesByDocKey(keys)

		for _, doc := range keys {
			lake, ok := pagesByDocKey[doc]
			if !ok {
				continue
			}
			brief := briefForSearchResult(lake.Text)

			values = append(values, SearchResult{
				Title: lake.Title,
				Score: docsScores[doc],
				Brief: brief,
				Url:   lake.Url,
			})
		}
	}

	latency := time.Since(t)
	c.HTML(200, "search.tpl", gin.H{
		"title":   "翰哥搜索",
		"time":    time.Now().Format("2006-01-02 15:04:05"),
		"values":  values,
		"keyword": keyword,
		"N":       N,
		"latency": latency,
	})

}

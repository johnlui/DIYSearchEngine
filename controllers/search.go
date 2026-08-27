package controllers

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

type SearchResult struct {
	Title string
	Score float64
	Brief string
	Url   string
}

var defaultSearchService = NewSearchService()

func Search(c *gin.Context) {
	t := time.Now()

	keyword := c.Query("keyword")

	result, err := defaultSearchService.Search(keyword)
	if err != nil {
		c.String(http.StatusInternalServerError, err.Error())
		return
	}

	latency := time.Since(t)
	c.HTML(200, "search.tpl", gin.H{
		"title":   "翰哥搜索",
		"time":    time.Now().Format("2006-01-02 15:04:05"),
		"values":  result.Values,
		"keyword": keyword,
		"N":       result.N,
		"latency": latency,
	})

}

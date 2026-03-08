package tools

import (
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/johnlui/enterprise-search-engine/db"
	"github.com/johnlui/enterprise-search-engine/models"
)

// 4 秒超时
var client = &http.Client{
	Timeout: time.Second * 4,
	CheckRedirect: func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	},
}

func Curl(status models.Status) (*goquery.Document, int) {
	req, err := http.NewRequest(http.MethodGet, status.Url, nil)
	if err != nil {
		return curlFailureResult(status)
	}
	req.Header.Set("User-Agent", "Sogou web spider/4.0(+http://www.sogou.com/docs/help/webmasters.htm#07)")

	resp, err := client.Do(req)
	if err != nil {
		return curlFailureResult(status)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil && !errors.Is(err, io.EOF) {
		document, _ := goquery.NewDocumentFromReader(strings.NewReader(""))
		return document, 3
	}

	doc, err := goquery.NewDocumentFromReader(strings.NewReader(string(body)))
	if err != nil {
		document, _ := goquery.NewDocumentFromReader(strings.NewReader(""))
		return document, 3
	}

	return doc, 1
}

var countCurlFailure = func(status models.Status) int {
	// 网络错误则使用Redis判断次数，达到3次则标记为 craw_done
	key := "ese_spider_wangluocuowu_" + GetMD5Hash(status.Url)

	count, err := db.Rdb.Get(db.Ctx, key).Int()
	if err == nil && count >= 2 { // 超时放弃次数
		return 4
	}

	db.Rdb.IncrBy(db.Ctx, key, 1).Err()
	db.Rdb.Expire(db.Ctx, key, time.Hour*240).Err()

	return 2
}

func curlFailureResult(status models.Status) (*goquery.Document, int) {
	document, _ := goquery.NewDocumentFromReader(strings.NewReader(""))
	return document, countCurlFailure(status)
}

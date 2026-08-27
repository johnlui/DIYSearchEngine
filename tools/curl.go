package tools

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
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
	return CurlContext(context.Background(), status)
}

func CurlContext(ctx context.Context, status models.Status) (*goquery.Document, int) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, status.Url, nil)
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

var countCurlFailure = defaultCurlFailureCounter

func SetCurlFailureCounter(fn func(models.Status) int) {
	if fn == nil {
		countCurlFailure = defaultCurlFailureCounter
		return
	}
	countCurlFailure = fn
}

func defaultCurlFailureCounter(models.Status) int {
	return 2
}

func curlFailureResult(status models.Status) (*goquery.Document, int) {
	document, _ := goquery.NewDocumentFromReader(strings.NewReader(""))
	return document, countCurlFailure(status)
}

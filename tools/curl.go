package tools

import (
  "fmt"
  "strings"
  "time"

  "github.com/PuerkitoBio/goquery"
  "github.com/imroc/req/v3"
  "github.com/johnlui/enterprise-search-engine/db"
  "github.com/johnlui/enterprise-search-engine/models"
)

// 4 秒超时
var client = req.C().SetTimeout(time.Second * 4).SetRedirectPolicy(req.NoRedirectPolicy())

func Curl(status models.Status, ch chan int) (*goquery.Document, int) {
  // Send a request with multiple headers.
  resp, err := client.R().
    SetHeader("User-Agent", "Sogou web spider/4.0(+http://www.sogou.com/docs/help/webmasters.htm#07)").
    Get(status.Url)
  if err != nil {
    // fmt.Println("网络错误：", url, err)
    // fmt.Println(err)
    document, _ := goquery.NewDocumentFromReader(strings.NewReader(""))

    // 网络错误则使用Redis判断次数，达到3次则标记为 craw_donw
    key := "ese_spider_wangluocuowu_" + GetMD5Hash(status.Url)

    count, err := db.Rdb.Get(db.Ctx, key).Int()
    if err == nil {
      if count >= 2 { // 超时放弃次数
        return document, 4
      }
    }

    db.Rdb.IncrBy(db.Ctx, key, 1).Err()
    db.Rdb.Expire(db.Ctx, key, time.Hour*240).Err()

    return document, 2
  }
  html := resp.String()
  doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
  if err != nil {
    fmt.Println("HTML解析失败：", status.Url, err)
    // fmt.Println(err)
    document, _ := goquery.NewDocumentFromReader(strings.NewReader(""))
    return document, 3
  }

  return doc, 1
}

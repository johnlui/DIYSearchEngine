package controllers

import (
  "fmt"
  "math"
  "os"
  "regexp"
  "sort"
  "strconv"
  "strings"
  "time"
  "unicode/utf8"

  "github.com/gin-gonic/gin"
  "github.com/johnlui/enterprise-search-engine/db"
  "github.com/johnlui/enterprise-search-engine/models"
  "github.com/johnlui/enterprise-search-engine/tools"
  "golang.org/x/exp/maps"
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

    r := tools.GetFenciResultArray(keyword)

    // var result []DocResult

    // 总文档数
    docsExistMap := make(map[string]struct{})

    // 总积分
    docsScores := make(map[string]float64)

    db.DbInstance0.Raw("select count(*) from pages_70 where dic_done = 1").Scan(&N)
    N *= 256
    if os.Getenv("APP_ENV") == "local" {
      N = 650000
    }
    if N == 0 {
      panic("文档总数N不能为零")
    }

    for _, v := range r {
      // 一个词
      var dic models.WordDic
      db.DbInstanceDic.Where("name = ?", v).Find(&dic)

      rawParts := strings.Split(dic.Positions, "-")
      rawParts = rawParts[:len(rawParts)-1]

      // 只保留同一个 docID count 较大的那个，同一个doc可能会重复出现在一个词中
      parts := make(map[string]string)
      for k, v := range rawParts {
        if k > 0 {

          intsV := strings.Split(v, ",")

          partsKey := intsV[0] + "~" + intsV[1]
          prsV, prs := parts[partsKey]
          if prs {
            prsIntsV := strings.Split(prsV, ",")
            prsVCount := prsIntsV[2]
            vCount := intsV[2]
            if vCount > prsVCount {
              parts[partsKey] = v
            }
          } else {
            parts[partsKey] = v
          }
        }
      }
      partsArr := maps.Values(parts)

      NQi := len(partsArr)
      IDF := math.Log10((float64(N-NQi) + 0.5) / (float64(NQi) + 0.5))

      wordExistCount := 0
      for _, p := range partsArr {
        // 一个词在一个文档里出现
        ints := strings.Split(p, ",")

        // 出现总次数
        Dj, _ := strconv.Atoi(ints[3])
        // 出现总次数
        Fi, err := strconv.Atoi(ints[2])
        // 出现总次数之和
        if err != nil {
          wordExistCount += Fi
        }

        // https://zhuanlan.zhihu.com/p/499906089

        k1 := 2.0
        b := 0.75
        // 平均文档长度，暂时没用，没有记录文档长度
        avgDocLength := 13214.0

        RQiDj := (float64(Fi) * (k1 + 1)) / (float64(Fi) + k1*(1-b+b*(float64(Dj)/avgDocLength)))

        docName := ints[0] + "-" + ints[1]
        _, prs := docsScores[docName]
        if !prs {
          docsScores[docName] = 0.0
        }

        docsScores[docName] += IDF * RQiDj

        // 总文档数
        _, prs1 := docsExistMap[docName]
        if !prs1 {
          docsExistMap[docName] = struct{}{}
        }

        // result := DocResult{
        //   count: 1,
        // }
      }

    }
    // dd(len(docsExistMap))
    // dd(docsScores)

    // 按照分数排序
    keys := make([]string, 0, len(docsScores))
    for key := range docsScores {
      keys = append(keys, key)
    }
    sort.SliceStable(keys, func(i, j int) bool {
      return docsScores[keys[i]] > docsScores[keys[j]]
    })

    // 取前10个
    qu := 200
    if len(keys) < qu {
      qu = len(keys)
    }
    keys = keys[0:qu]

    for _, doc := range keys {
      ps := strings.Split(doc, "-")

      tableIndex, _ := strconv.Atoi(ps[0])
      var tableName string
      if tableIndex < 16 {
        tableName = fmt.Sprintf("pages_0%x", tableIndex)
      } else {
        tableName = fmt.Sprintf("pages_%x", tableIndex)
      }

      realDB := db.DbInstance0

      // 如果你有多个数据库，可以取消注释
      // if tableIndex > 127 {
      //   realDB = db.DbInstance1
      // }

      var lake models.Page
      realDB.Table(tableName).Where("id = ?", ps[1]).Scan(&lake)

      // fmt.Println(lake.Title, docsScores[doc])

      re := regexp.MustCompile("[[:ascii:]]")
      brief := re.ReplaceAllLiteralString(lake.Text, "")

      length := 100
      briefLen := utf8.RuneCountInString(brief)
      if briefLen < 100 {
        length = briefLen
      }
      if length > 0 {
        brief = string([]rune(brief)[:length-1])
      }

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

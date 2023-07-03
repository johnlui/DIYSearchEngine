package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/johnlui/enterprise-search-engine/db"
	"github.com/johnlui/enterprise-search-engine/models"
	"github.com/johnlui/enterprise-search-engine/tools"
	"golang.org/x/text/width"
	"gorm.io/gorm"
)

// 后台定时自动同步 pages 表到 status 表
func autoParsePagesToStatus() {
  t := time.Now()

  var count int64 = 0

  realDB := db.DbInstance0

  for i := 0; i < 256; i++ {
    var pagesTableName string
    var statusTableName string
    if i < 16 {
      pagesTableName = fmt.Sprintf("pages_0%x", i)
      statusTableName = fmt.Sprintf("status_0%x", i)
    } else {
      pagesTableName = fmt.Sprintf("pages_%x", i)
      statusTableName = fmt.Sprintf("status_%x", i)
    }

    result := realDB.Exec("insert into `" + statusTableName + "` select `id`, `url`, `host`, `craw_done`, `craw_time` from `" + pagesTableName + "` where id > COALESCE((select max(id) from status_00), 0);")

    count += result.RowsAffected
  }
  if count > 0 {
    fmt.Println("从 pages 同步了一批数据到 status", time.Now().Unix()-t.Unix(), "秒，共", count, "条")
  }
}

// 定时将可以爬的 URL 从 status 表转移到 redis 中
func prepareStatusesBackground() {
  t := time.Now()

  maxNumber := 1
  if os.Getenv("APP_DEBUG") == "false" {
    maxNumber = 一次准备
  }

  // host 黑名单，用于提升过滤效率
  hostBlackListInOneStepArray, _ := db.Rdb.SMembers(db.Ctx, "ese_spider_host_black_list").Result()
  if len(hostBlackListInOneStepArray) == 0 {
    db.Rdb.SAdd(db.Ctx, "ese_spider_host_black_list", "ooxx")
    db.Rdb.Expire(db.Ctx, "ese_spider_host_black_list", time.Minute*42).Err()
    for k := range domain1BlackList {
      hostBlackListInOneStepArray = append(hostBlackListInOneStepArray, k)
    }
  }

  count := 0

  for i := 0; i < 256; i++ {
    var tableName string
    if i < 16 {
      tableName = fmt.Sprintf("status_0%x", i)
    } else {
      tableName = fmt.Sprintf("status_%x", i)
    }

    realDB := db.DbInstance0

    var _statusArray []models.Status
    key := "table_" + tableName + "_max_into_queue_id"
    maxID, _ := db.Rdb.Get(db.Ctx, key).Int()
    realDB.Table(tableName).
      Where("craw_done", 0).
      Where("host not in (?)", hostBlackListInOneStepArray).
      Where("id > ?", maxID).
      Order("id").Limit(maxNumber).Find(&_statusArray)

    if len(_statusArray) > 0 {
      count += len(_statusArray)

      for _, v := range _statusArray {
        taskBytes, _ := json.Marshal(v)
        db.Rdb.LPush(db.Ctx, "need_craw_list", taskBytes)
      }

      keyTTL, _ := db.Rdb.TTL(db.Ctx, key).Result()
      if keyTTL == -1 {
        keyTTL = time.Hour
      }

      err := db.Rdb.Set(db.Ctx, key, _statusArray[len(_statusArray)-1].ID, keyTTL).Err()
      if err != nil {
        dd(err)
      }
    }

  }

  if count > 0 {
    fmt.Println("准备完一轮数据", time.Now().Unix()-t.Unix(), "秒，共", maxNumber*256, "条")
  }
}

// 每天刷新一次 已爬 host 数量
func refreshHostCount() {
  t := time.Now()
  fmt.Println("开始刷新URL数")

  minutesInDay := t.Hour()*60 + t.Minute()

  start := minutesInDay / 5
  end := start + 1

  if start > 255 || end > 255 {
    return
  }

  // 总数
  for i := start; i < end; i++ {
    var tableName string
    if i < 16 {
      tableName = fmt.Sprintf("status_0%x", i)
    } else {
      tableName = fmt.Sprintf("status_%x", i)
    }

    realDB := db.DbInstance0
    _hostCountArr := []models.HostCount{}
    realDB.Raw("select host, count(*) count from " + tableName + " where host is not null group by host having count > 500").Scan(&_hostCountArr)

    key := "host_counts_all_" + strconv.Itoa(int(time.Now().Unix())/86400)
    for _, v := range _hostCountArr {
      db.Rdb.HIncrBy(db.Ctx, key, v.Host, int64(v.Count))
    }
    db.Rdb.Expire(db.Ctx, key, time.Hour*48).Err()
  }

  // 已爬数量
  for i := start; i < end; i++ {
    var tableName string
    if i < 16 {
      tableName = fmt.Sprintf("status_0%x", i)
    } else {
      tableName = fmt.Sprintf("status_%x", i)
    }

    realDB := db.DbInstance0
    _hostCountArr := []models.HostCount{}
    realDB.Raw("select host, count(*) crawd_count from " + tableName + " where craw_done = 1 and host is not null group by host").Scan(&_hostCountArr)

    key := "host_counts_crawd_" + strconv.Itoa(int(time.Now().Unix())/86400)
    for _, v := range _hostCountArr {
      db.Rdb.HIncrBy(db.Ctx, key, v.Host, int64(v.CrawdCount))
    }
    db.Rdb.Expire(db.Ctx, key, time.Hour*48).Err()
  }

  // 已爬但无效的数量
  for i := start; i < end; i++ {
    var tableName string
    if i < 16 {
      tableName = fmt.Sprintf("pages_0%x", i)
    } else {
      tableName = fmt.Sprintf("pages_%x", i)
    }

    realDB := db.DbInstance0
    _hostCountArr := []models.HostCount{}
    realDB.Raw("select host, count(*) crawd_count from " + tableName + " where craw_done = 1 and text = '' and host is not null group by host").Scan(&_hostCountArr)

    key := "host_counts_crawd_invalid_" + strconv.Itoa(int(time.Now().Unix())/86400)
    for _, v := range _hostCountArr {
      db.Rdb.HIncrBy(db.Ctx, key, v.Host, int64(v.CrawdCount))
    }
    db.Rdb.Expire(db.Ctx, key, time.Hour*48).Err()
  }

  fmt.Println("刷新URL数完成：start", start, "end", end, time.Now().Unix()-t.Unix(), "秒")
}

// 将分词结果洗到 redis DB10 里面
func washHTMLToDB10() {
  t := time.Now()
  chs := make([]chan int, 256)
  for i := 0; i < 256; i++ {
    var tableName string
    // var statusTableName string
    if i < 16 {
      tableName = fmt.Sprintf("pages_0%x", i)
      // statusTableName = fmt.Sprintf("status_0%x", i)
    } else {
      tableName = fmt.Sprintf("pages_%x", i)
      // statusTableName = fmt.Sprintf("status_%x", i)
    }

    realDB := db.DbInstance0

    chs[i] = make(chan int)
    go asyncGenerateDics(i, realDB, tableName, chs[i])
  }
  total := 0
  for _, ch := range chs {
    total += <-ch
  }

  if total > 0 {
    fmt.Println("将分词结果洗到 redis 里完成", time.Now().Unix()-t.Unix(), "秒", total, "条，启动时间", t.Format("2006-01-02 15:04:05"))
  }

  // 刷新字符黑名单

  _wordBlackList := []string{}
  db.DbInstance0.Raw("select word from word_black_list").Scan(&_wordBlackList)
  wordBlackList = make(map[string]struct{})
  for _, v := range _wordBlackList {
    wordBlackList[v] = struct{}{}
  }
}

type WordAndSppendSrting struct {
  word         string
  appendString string
}

// 将 redis 里的分词结果洗到数据库里
func washDB10ToDicMySQL() {
  _stop := -1
  db.DbInstance0.Table("kvstores").Where("k", "stopWashDicRedisToMySQL").Select("v").Find(&_stop)
  if _stop == -1 {
    fmt.Println("kvstores数据库连接失败，请检查 gorm-log.txt 日志")
    os.Exit(0)
  } else if _stop == 1 {
    fmt.Println("全局开关关闭，60秒后再检测")
    time.Sleep(time.Second * 60)
    washDB10ToDicMySQL()
  }

  fmt.Println("新的一轮")

  // 从 redis DB10 获取字典插入数据库
  // 1. 随机获取一个 key
  // 2. 判断长度，大于1，则保留最后一条，循环取出前面所有条
  // 3. 每次处理 100 个？ key
  // 4. 在 DB0 里面存一个 Hash：存储所有已经入库的词
  // 5. 插入之前监测一下词是否已入库，若从未入库，则执行创建语句，若已入库，跳过
  // 6. 使用事务批量执行 update
  needUpdate := make(map[string]string)
  t := time.Now()
  oneStep := 一步转移的字典条数

  chs := make([]chan WordAndSppendSrting, oneStep)

  for j := 0; j < oneStep; j++ {
    chs[j] = make(chan WordAndSppendSrting)
    go asyncGetWordAndSppendSrting(chs[j])
  }

  for _, ch := range chs {
    _result := <-ch
    if _result.word != "" {
      _, prs := needUpdate[_result.word]
      if prs {
        needUpdate[_result.word] += _result.appendString
      } else {
        needUpdate[_result.word] = _result.appendString
      }
    }
  }
  fmt.Println("开始插入数据库")
  db.DbInstanceDic.Connection(func(tx *gorm.DB) error {
    tx.Exec(`START TRANSACTION`)

    for w, s := range needUpdate {
      tx.Exec(`UPDATE word_dics
      SET positions = concat(ifnull(positions,''), ?) where name = ?`, s, w)
    }

    tx.Exec(`COMMIT`)

    return nil
  })

  if len(needUpdate) > 0 {
    fmt.Println("转移完一批字典，共", len(needUpdate), "条，启动时间", t.Format("2006-01-02 15:04:05"))
  }

  if len(needUpdate) > 0 {
    washDB10ToDicMySQL()
  } else {
    fmt.Println("全转移完啦！")
  }
}

func asyncGetWordAndSppendSrting(ch chan WordAndSppendSrting) {
  wordAndSppendSrting := WordAndSppendSrting{}

  word := db.Rdb10.RandomKey(db.Ctx).Val()
  len := db.Rdb10.LLen(db.Ctx, word).Val()
  if len > 0 {
    // fmt.Println(word, "长度", len)
    if !db.Rdb.HExists(db.Ctx, "HasBeenTransported", word).Val() {
      db.DbInstanceDic.Exec(`INSERT IGNORE INTO word_dics
                        SET name = ?,
                        positions = ''`, word)
    }
    db.Rdb.HSet(db.Ctx, "HasBeenTransported", word, "")

    stringNeedAdd := ""
    var i int64 = 0
    for i < len {
      if i >= 每个词转移的深度 {
        break
      }
      stringNeedAdd += db.Rdb10.LPop(db.Ctx, word).Val()
      i += 1
    }
    wordAndSppendSrting.word = word
    wordAndSppendSrting.appendString = stringNeedAdd
  }

  ch <- wordAndSppendSrting
}
func asyncGenerateDics(i int, realDB *gorm.DB, tableName string, ch chan int) {
  var lakes []models.Page
  realDB.Table(tableName).
    Where("dic_done = 0").
    Where("craw_done = 1").
    Order("id asc").
    Limit(每分钟每个表执行分词).
    Scan(&lakes)
  // tools.DD(lakes[0].Text)

  /*
     1. 分词，然后对分词结果进行重整：
     2. 统计词频
     3. 计算出 文档号,位置 ，可能存在多个
     4. 创建词或者 update ：update tablename set col1name = concat(ifnull(col1name,""), 'a,b,c');
     5. 处理成单字，另存一份倒排索引字典
  */
  for _, lake := range lakes {
    text := lake.Text
    textLength := utf8.RuneCountInString(text)

    r := tools.GetFenciResultArray(text)
    // tools.DD(r)

    // 计算位置+统计词频
    uniqueWordResult := make(map[string]WordResult)
    position := 0
    for _, w := range r {
      // 转半角
      word := width.Narrow.String(w)
      length := utf8.RuneCountInString(word)

      _, pr := wordBlackList[word]
      if pr {
        continue
      }

      _, prs := uniqueWordResult[word]
      if !prs {
        uniqueWordResult[word] = WordResult{
          count:     1,
          positions: []string{strconv.Itoa(position)},
        }
      } else {
        uniqueWordResult[word] = WordResult{
          count:     uniqueWordResult[word].count + 1,
          positions: append(uniqueWordResult[word].positions, strconv.Itoa(position)),
        }
      }

      position += length
    }

    for w, v := range uniqueWordResult {
      appendSrting := strconv.Itoa(i) + "," +
        strconv.Itoa(int(lake.ID)) + "," +
        strconv.Itoa(v.count) + "," +
        strconv.Itoa(textLength) + "," +
        strings.Join(v.positions, ",") +
        "-"

      db.Rdb10.RPush(db.Ctx, w, appendSrting)

    }

    lake.DicDone = 1
    realDB.Table(tableName).Save(&lake)
  }

  ch <- len(lakes)
}

type WordResult struct {
  count     int
  positions []string
}

package main

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/johnlui/enterprise-search-engine/internal/logging"
	"github.com/johnlui/enterprise-search-engine/internal/storage"
	"github.com/johnlui/enterprise-search-engine/tools"
	"golang.org/x/text/width"
	"gorm.io/gorm"
)

var currentTime = time.Now

// 后台定时自动同步 pages 表到 status 表
func autoParsePagesToStatus() {
	t := currentTime()

	var count int64 = 0

	for i := 0; i < 256; i++ {
		rowsAffected, err := storage.FromGlobals().SyncPagesToStatus(i)
		if err != nil {
			logging.Errorf("event=sync_pages_to_status shard=%d error=%q", i, err)
			continue
		}
		count += rowsAffected
	}
	if count > 0 {
		fmt.Println("从 pages 同步了一批数据到 status", currentTime().Unix()-t.Unix(), "秒，共", count, "条")
	}
}

// 定时将可以爬的 URL 从 status 表转移到 redis 中
func prepareStatusesBackground() {
	t := currentTime()

	maxNumber := 1
	if os.Getenv("APP_DEBUG") == "false" {
		maxNumber = 一次准备
	}

	// host 黑名单，用于提升过滤效率
	domain1BlackList = loadCurrentDomainBlacklist()
	hostBlackListInOneStepArray := storage.FromGlobals().LoadHostBlacklist(domain1BlackList)

	count := runWorkerPool(256, 8, func(i int) int {
		return enqueueStatusesForTable(i, maxNumber, hostBlackListInOneStepArray)
	})

	if count > 0 {
		fmt.Println("准备完一轮数据", currentTime().Unix()-t.Unix(), "秒，共", maxNumber*256, "条")
	}
}

// 每天刷新一次 已爬 host 数量
func refreshHostCount() {
	t := currentTime()
	fmt.Println("开始刷新URL数")

	minutesInDay := t.Hour()*60 + t.Minute()

	start := minutesInDay / 5
	end := start + 1

	if start > 255 || end > 255 {
		return
	}

	for i := start; i < end; i++ {
		if err := storage.FromGlobals().RefreshHostCountsForShard(i, currentTime()); err != nil {
			logging.Errorf("event=refresh_host_counts shard=%d error=%q", i, err)
		}
	}

	fmt.Println("刷新URL数完成：start", start, "end", end, currentTime().Unix()-t.Unix(), "秒")
}

// 将分词结果洗到 redis DB10 里面
func washHTMLToDB10() {
	t := currentTime()
	total := runWorkerPool(256, 16, func(i int) int {
		return generateDicsForTableWithStore(i, storage.FromGlobals(), tools.HexTableName("pages", i))
	})

	if total > 0 {
		fmt.Println("将分词结果洗到 redis 里完成", currentTime().Unix()-t.Unix(), "秒", total, "条，启动时间", t.Format("2006-01-02 15:04:05"))
	}

	reloadWordBlacklist()
}

type WordAndSppendSrting struct {
	word         string
	appendString string
}

// 将 redis 里的分词结果洗到数据库里
func washDB10ToDicMySQL() {
	for {
		_stop, err := readKVInt("stopWashDicRedisToMySQL")
		if err != nil {
			logging.Errorf("event=read_control_flag flag=stopWashDicRedisToMySQL action=retry error=%q", err)
			sleep(time.Second * 60)
			continue
		}
		if _stop == 1 {
			fmt.Println("全局开关关闭，60秒后再检测")
			sleep(time.Second * 60)
			continue
		}

		fmt.Println("新的一轮")
		if !transferWordDicsBatch() {
			fmt.Println("全转移完啦！")
			return
		}
	}
}

func transferWordDicsBatch() bool {
	// 从 redis DB10 获取字典插入数据库
	// 1. 随机获取一个 key
	// 2. 判断长度，大于1，则保留最后一条，循环取出前面所有条
	// 3. 每次处理 100 个？ key
	// 4. 在 DB0 里面存一个 Hash：存储所有已经入库的词
	// 5. 插入之前监测一下词是否已入库，若从未入库，则执行创建语句，若已入库，跳过
	// 6. 使用事务批量执行 update
	needUpdate := make(map[string]string)
	t := currentTime()
	oneStep := 一步转移的字典条数

	var mu sync.Mutex
	runWorkerPool(oneStep, 32, func(_ int) int {
		result := getWordAndSppendSrting()
		if result.word == "" {
			return 0
		}

		mu.Lock()
		needUpdate[result.word] += result.appendString
		mu.Unlock()
		return 1
	})

	fmt.Println("开始插入数据库")
	if err := storage.FromGlobals().SaveWordAppends(needUpdate); err != nil {
		dd(err)
	}

	if len(needUpdate) > 0 {
		fmt.Println("转移完一批字典，共", len(needUpdate), "条，启动时间", t.Format("2006-01-02 15:04:05"))
	}

	return len(needUpdate) > 0
}

func getWordAndSppendSrting() WordAndSppendSrting {
	wordAndSppendSrting := WordAndSppendSrting{}

	append := storage.FromGlobals().PopWordAppend(每个词转移的深度)
	wordAndSppendSrting.word = append.Word
	wordAndSppendSrting.appendString = append.Payload
	return wordAndSppendSrting
}

func generateDicsForTable(i int, realDB *gorm.DB, tableName string) int {
	store := storage.FromGlobals()
	store.PagesDB = realDB
	return generateDicsForTableWithStore(i, store, tableName)
}

func generateDicsForTableWithStore(i int, store storage.Handles, tableName string) int {
	lakes := store.LoadPagesForIndex(tableName, 每分钟每个表执行分词)
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

			store.PushIndexAppend(w, appendSrting)

		}

		store.MarkPageIndexed(tableName, lake)
	}

	return len(lakes)
}

type WordResult struct {
	count     int
	positions []string
}

func enqueueStatusesForTable(i, maxNumber int, hostBlackListInOneStepArray []string) int {
	return storage.FromGlobals().EnqueueStatusesForTable(i, maxNumber, hostBlackListInOneStepArray)
}

func reloadWordBlacklist() {
	wordBlackList = storage.FromGlobals().LoadWordBlacklist()
}

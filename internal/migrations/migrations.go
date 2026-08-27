package migrations

import (
	"time"

	"github.com/johnlui/enterprise-search-engine/models"
	"github.com/johnlui/enterprise-search-engine/tools"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type DomainBlacklist struct {
	ID     uint   `gorm:"primaryKey"`
	Domain string `gorm:"uniqueIndex;size:255"`
}

type WordBlacklist struct {
	ID   uint   `gorm:"primaryKey"`
	Word string `gorm:"uniqueIndex;size:255"`
}

type KVStore struct {
	ID   uint      `gorm:"primaryKey"`
	K    string    `gorm:"uniqueIndex;size:255"`
	V    string    `gorm:"size:255"`
	Time time.Time `gorm:"default:'2001-01-01 00:00:01'"`
}

func Run(pagesDB, dictionaryDB *gorm.DB) error {
	if err := createShardTables(pagesDB); err != nil {
		return err
	}
	if err := createControlTables(pagesDB); err != nil {
		return err
	}
	if err := seedControlTables(pagesDB); err != nil {
		return err
	}
	return createDictionaryTables(dictionaryDB)
}

func createShardTables(db *gorm.DB) error {
	for i := 0; i < 256; i++ {
		if err := db.Table(tools.HexTableName("pages", i)).AutoMigrate(&models.Page{}); err != nil {
			return err
		}
		if err := db.Table(tools.HexTableName("status", i)).AutoMigrate(&models.Status{}); err != nil {
			return err
		}
	}
	return nil
}

func createControlTables(db *gorm.DB) error {
	if err := db.Table("domain_black_list").AutoMigrate(&DomainBlacklist{}); err != nil {
		return err
	}
	if err := db.Table("word_black_list").AutoMigrate(&WordBlacklist{}); err != nil {
		return err
	}
	return db.Table("kvstores").AutoMigrate(&KVStore{})
}

func createDictionaryTables(db *gorm.DB) error {
	if err := db.Table("word_dics").AutoMigrate(&models.WordDic{}); err != nil {
		return err
	}
	return db.AutoMigrate(&models.WordPosting{})
}

func seedControlTables(db *gorm.DB) error {
	domains := []DomainBlacklist{
		{ID: 1, Domain: "huangye88.com"},
		{ID: 2, Domain: "gov.cn"},
		{ID: 3, Domain: "nbhesen.com"},
		{ID: 4, Domain: "tianyancha.com"},
		{ID: 5, Domain: "qianlima.com"},
		{ID: 6, Domain: "99114.com"},
		{ID: 7, Domain: "luosi.com"},
		{ID: 8, Domain: "bidchance.com"},
		{ID: 9, Domain: "51zhantai.com"},
		{ID: 10, Domain: "baiye5.com"},
		{ID: 11, Domain: "snxx.com"},
		{ID: 12, Domain: "6789go.com"},
		{ID: 13, Domain: "gongxiangchi.com"},
		{ID: 14, Domain: "webacg.com"},
		{ID: 16, Domain: "912688.com"},
		{ID: 17, Domain: "dihe.cn"},
		{ID: 18, Domain: "maoyihang.com"},
		{ID: 19, Domain: "realsee.com"},
		{ID: 20, Domain: "tdzyw.com"},
		{ID: 21, Domain: "anjuke.com"},
		{ID: 22, Domain: "liuxue86.com"},
		{ID: 23, Domain: "5588.tv"},
		{ID: 24, Domain: "58.com"},
	}
	if err := db.Table("domain_black_list").Clauses(clause.OnConflict{DoNothing: true}).Create(&domains).Error; err != nil {
		return err
	}

	words := []WordBlacklist{
		{ID: 1, Word: "px"}, {ID: 2, Word: "20"}, {ID: 3, Word: "("}, {ID: 4, Word: ")"},
		{ID: 5, Word: ","}, {ID: 6, Word: "."}, {ID: 7, Word: "-"}, {ID: 8, Word: "/"},
		{ID: 9, Word: ":"}, {ID: 10, Word: "var"}, {ID: 11, Word: "的"}, {ID: 12, Word: "com"},
		{ID: 13, Word: ";"}, {ID: 14, Word: "["}, {ID: 15, Word: "]"}, {ID: 16, Word: "{"},
		{ID: 17, Word: "}"}, {ID: 18, Word: "'"}, {ID: 19, Word: `"`}, {ID: 20, Word: "_"},
		{ID: 21, Word: "?"}, {ID: 22, Word: "function"}, {ID: 23, Word: "document"}, {ID: 24, Word: "|"},
		{ID: 25, Word: "="}, {ID: 26, Word: "html"}, {ID: 27, Word: "内容"}, {ID: 28, Word: "0"},
		{ID: 29, Word: "1"}, {ID: 30, Word: "3"}, {ID: 31, Word: "https"}, {ID: 32, Word: "http"},
		{ID: 33, Word: "2"}, {ID: 34, Word: "!"}, {ID: 35, Word: "window"}, {ID: 36, Word: "if"},
		{ID: 37, Word: "“"}, {ID: 38, Word: "”"}, {ID: 39, Word: "。"}, {ID: 40, Word: "src"},
		{ID: 41, Word: "中"}, {ID: 42, Word: "了"}, {ID: 43, Word: "6"}, {ID: 44, Word: "｡"},
		{ID: 45, Word: "<"}, {ID: 46, Word: ">"}, {ID: 47, Word: "联系"}, {ID: 48, Word: "号"},
		{ID: 49, Word: "getElementsByTagName"}, {ID: 50, Word: "5"}, {ID: 51, Word: "､"},
		{ID: 52, Word: "script"}, {ID: 53, Word: "js"},
	}
	if err := db.Table("word_black_list").Clauses(clause.OnConflict{DoNothing: true}).Create(&words).Error; err != nil {
		return err
	}

	flags := []KVStore{
		{ID: 1, K: "stop", V: "0", Time: time.Date(2022, 9, 4, 1, 27, 55, 0, time.Local)},
		{ID: 2, K: "stopNew", V: "0", Time: time.Date(2001, 1, 1, 0, 0, 1, 0, time.Local)},
		{ID: 3, K: "stopWashDicRedisToMySQL", V: "0", Time: time.Date(2001, 1, 1, 0, 0, 1, 0, time.Local)},
	}
	return db.Table("kvstores").Clauses(clause.OnConflict{DoNothing: true}).Create(&flags).Error
}

package storage

import (
	"github.com/go-redis/redis/v8"
	"github.com/johnlui/enterprise-search-engine/db"
	"github.com/johnlui/enterprise-search-engine/tools"
	"gorm.io/gorm"
)

type Handles struct {
	PagesDB      *gorm.DB
	DictionaryDB *gorm.DB
	Redis        *redis.Client
	IndexRedis   *redis.Client
}

func FromGlobals() Handles {
	return Handles{
		PagesDB:      db.DbInstance0,
		DictionaryDB: db.DbInstanceDic,
		Redis:        db.Rdb,
		IndexRedis:   db.Rdb10,
	}
}

func (h Handles) DBForURL(string) *gorm.DB {
	return h.PagesDB
}

func (h Handles) StatusTableForURL(url string) string {
	return tools.MD5TableName("status", url)
}

func (h Handles) PageTableForURL(url string) string {
	return tools.MD5TableName("pages", url)
}

func (h Handles) StatusTable(index int) string {
	return tools.HexTableName("status", index)
}

func (h Handles) PageTable(index int) string {
	return tools.HexTableName("pages", index)
}

func (h Handles) TableScope(prefix, value string) func(tx *gorm.DB) *gorm.DB {
	tableName := tools.MD5TableName(prefix, value)
	return func(tx *gorm.DB) *gorm.DB {
		return tx.Table(tableName)
	}
}

func (h Handles) StatusScope(url string) func(tx *gorm.DB) *gorm.DB {
	return h.TableScope("status", url)
}

func (h Handles) PageScope(url string) func(tx *gorm.DB) *gorm.DB {
	return h.TableScope("pages", url)
}

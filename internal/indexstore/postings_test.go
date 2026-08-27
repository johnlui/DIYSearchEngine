package indexstore

import (
	"testing"

	"github.com/johnlui/enterprise-search-engine/models"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestParsePostings(t *testing.T) {
	postings := ParsePostings("hello", "0,1,3,100,0,4-1,2,1,80,7-ignored-bad,part-")
	if len(postings) != 2 {
		t.Fatalf("postings len = %d, want 2: %#v", len(postings), postings)
	}
	if postings[0].Term != "hello" || postings[0].TableIndex != 0 || postings[0].DocID != 1 ||
		postings[0].TermFrequency != 3 || postings[0].DocLength != 100 || postings[0].Positions != "0,4" {
		t.Fatalf("first posting = %#v", postings[0])
	}
	if got := ParsePostings("", "0,1,1,1-"); got != nil {
		t.Fatalf("empty term postings = %#v", got)
	}
	if got := ParsePostings("hello", "0,bad,1,1-0,1,bad,1-0,1,1,bad-"); len(got) != 0 {
		t.Fatalf("invalid postings = %#v", got)
	}
}

func TestAppendAndLoadPostings(t *testing.T) {
	dbInstance, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := dbInstance.AutoMigrate(&models.WordPosting{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	if err := AppendPostings(dbInstance, "hello", "0,1,3,100,0-"); err != nil {
		t.Fatalf("append postings: %v", err)
	}
	if err := AppendPostings(dbInstance, "hello", "bad"); err != nil {
		t.Fatalf("append empty postings: %v", err)
	}
	if err := AppendPostings(dbInstance, "hello", "0,1,5,120,1-"); err != nil {
		t.Fatalf("append postings update: %v", err)
	}

	loaded, err := LoadPostings(dbInstance, []string{"hello", "missing"})
	if err != nil {
		t.Fatalf("load postings: %v", err)
	}
	if len(loaded["hello"]) != 1 {
		t.Fatalf("loaded hello = %#v", loaded["hello"])
	}
	if loaded["hello"][0].TermFrequency != 5 || loaded["hello"][0].DocLength != 120 {
		t.Fatalf("updated posting = %#v", loaded["hello"][0])
	}
	empty, err := LoadPostings(dbInstance, nil)
	if err != nil || len(empty) != 0 {
		t.Fatalf("LoadPostings empty = %#v, %v", empty, err)
	}
}

func TestAppendLegacyWordDic(t *testing.T) {
	dbInstance, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := dbInstance.Table("word_dics").AutoMigrate(&models.WordDic{}); err != nil {
		t.Fatalf("migrate word_dics: %v", err)
	}
	if err := EnsureWordDic(dbInstance, "hello"); err != nil {
		t.Fatalf("ensure word dic: %v", err)
	}
	if err := EnsureWordDic(dbInstance, ""); err != nil {
		t.Fatalf("ensure empty word dic: %v", err)
	}
	if err := AppendLegacyWordDic(dbInstance, "hello", "0,1,1,8,0-"); err != nil {
		t.Fatalf("append legacy: %v", err)
	}
	if err := AppendLegacyWordDic(dbInstance, "", "payload"); err != nil {
		t.Fatalf("append empty legacy term: %v", err)
	}
	if err := AppendLegacyWordDic(dbInstance, "hello", ""); err != nil {
		t.Fatalf("append empty legacy payload: %v", err)
	}

	var dic models.WordDic
	if err := dbInstance.Table("word_dics").Where("name = ?", "hello").First(&dic).Error; err != nil {
		t.Fatalf("load word dic: %v", err)
	}
	if dic.Positions != "0,1,1,8,0-" {
		t.Fatalf("positions = %q", dic.Positions)
	}
}

func TestLoadPostingsWithoutTable(t *testing.T) {
	dbInstance, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}

	postings, err := LoadPostings(dbInstance, []string{"hello"})
	if err != nil {
		t.Fatalf("LoadPostings without table: %v", err)
	}
	if len(postings) != 0 {
		t.Fatalf("postings without table = %#v", postings)
	}
}

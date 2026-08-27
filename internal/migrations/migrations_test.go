package migrations

import (
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestRunCreatesSchemaAndSeeds(t *testing.T) {
	dbInstance, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}

	if err := Run(dbInstance, dbInstance); err != nil {
		t.Fatalf("run migrations: %v", err)
	}

	for _, table := range []string{"pages_00", "status_00", "domain_black_list", "word_black_list", "kvstores", "word_dics", "word_postings"} {
		if !dbInstance.Migrator().HasTable(table) {
			t.Fatalf("expected table %s", table)
		}
	}

	var stopValue string
	if err := dbInstance.Table("kvstores").Where("k = ?", "stop").Select("v").Scan(&stopValue).Error; err != nil {
		t.Fatalf("load stop flag: %v", err)
	}
	if stopValue != "0" {
		t.Fatalf("stop flag = %q", stopValue)
	}
}

func openClosedSQLite(t *testing.T) *gorm.DB {
	t.Helper()

	dbInstance, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	sqlDB, err := dbInstance.DB()
	if err != nil {
		t.Fatalf("sqlite DB handle: %v", err)
	}
	if err := sqlDB.Close(); err != nil {
		t.Fatalf("close sqlite: %v", err)
	}
	return dbInstance
}

func TestMigrationErrorsBubbleUp(t *testing.T) {
	healthyDB, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open healthy sqlite: %v", err)
	}
	if err := Run(openClosedSQLite(t), healthyDB); err == nil {
		t.Fatal("expected Run to return shard creation error")
	}
	if err := createControlTables(openClosedSQLite(t)); err == nil {
		t.Fatal("expected createControlTables to return error")
	}
	if err := createDictionaryTables(openClosedSQLite(t)); err == nil {
		t.Fatal("expected createDictionaryTables to return error")
	}
	if err := seedControlTables(openClosedSQLite(t)); err == nil {
		t.Fatal("expected seedControlTables to return error")
	}
}

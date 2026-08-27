package db

import (
	"reflect"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/go-redis/redis/v8"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func openSQLiteForTest(t *testing.T) *gorm.DB {
	t.Helper()

	dbInstance, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	return dbInstance
}

func TestInitDBUsesEnvironmentAndInitializers(t *testing.T) {
	originalOpenDB := openDB
	originalConfigurePool := configurePool
	originalCreateRedisClient := createRedisClient
	originalPingRedis := pingRedis
	defer func() {
		openDB = originalOpenDB
		configurePool = originalConfigurePool
		createRedisClient = originalCreateRedisClient
		pingRedis = originalPingRedis
	}()

	t.Setenv("APP_DEBUG", "true")
	t.Setenv("DB_USERNAME0", "user0")
	t.Setenv("DB_PASSWORD0", "pass0")
	t.Setenv("DB_HOST0", "host0")
	t.Setenv("DB_PORT0", "3306")
	t.Setenv("DB_DATABASE0", "pages")
	t.Setenv("DB_USERNAME_DIC", "userd")
	t.Setenv("DB_PASSWORD_DIC", "passd")
	t.Setenv("DB_HOST_DIC", "hostd")
	t.Setenv("DB_PORT_DIC", "3307")
	t.Setenv("DB_DATABASE_DIC", "dic")

	opened := map[string]string{}
	openDB = func(name, dsn string, _ *gorm.Config) *gorm.DB {
		opened[name] = dsn
		return openSQLiteForTest(t)
	}

	configured := []string{}
	configurePool = func(name string, _ *gorm.DB, maxIdle, maxOpen int) {
		configured = append(configured, name)
		if maxIdle != 1 || maxOpen != 20 {
			t.Fatalf("configurePool(%s) = %d/%d", name, maxIdle, maxOpen)
		}
	}

	createRedisClient = func(dbIndex int) *redis.Client {
		return redis.NewClient(&redis.Options{Addr: "127.0.0.1:1", DB: dbIndex})
	}

	pinged := []string{}
	pingRedis = func(name string, _ *redis.Client) {
		pinged = append(pinged, name)
	}

	InitDB()

	if opened["pages"] != "user0:pass0@(host0:3306)/pages?charset=utf8mb4&parseTime=True&loc=Local" {
		t.Fatalf("pages DSN = %q", opened["pages"])
	}
	if opened["dictionary"] != "userd:passd@(hostd:3307)/dic?charset=utf8mb4&parseTime=True&loc=Local" {
		t.Fatalf("dictionary DSN = %q", opened["dictionary"])
	}
	if !reflect.DeepEqual(configured, []string{"pages", "dictionary"}) {
		t.Fatalf("configured = %#v", configured)
	}
	if !reflect.DeepEqual(pinged, []string{"redis-0", "redis-10"}) {
		t.Fatalf("pinged = %#v", pinged)
	}
	if DbInstance0 == nil || DbInstanceDic == nil || Rdb == nil || Rdb10 == nil {
		t.Fatal("expected InitDB to assign global handles")
	}
}

func TestConfigureDBPoolWithSQLite(t *testing.T) {
	configureDBPool("sqlite", openSQLiteForTest(t), 2, 4)
}

func TestMustOpenDBSuccess(t *testing.T) {
	originalMakeMysqlDialector := makeMysqlDialector
	originalOpenGorm := openGorm
	defer func() {
		makeMysqlDialector = originalMakeMysqlDialector
		openGorm = originalOpenGorm
	}()

	seenDSN := ""
	makeMysqlDialector = func(dsn string) gorm.Dialector {
		seenDSN = dsn
		return sqlite.Open(":memory:")
	}
	openGorm = func(dialector gorm.Dialector, opts ...gorm.Option) (*gorm.DB, error) {
		if len(opts) != 1 {
			t.Fatalf("openGorm opts len = %d", len(opts))
		}
		return gorm.Open(dialector, opts...)
	}

	dbInstance := mustOpenDB("test", "dsn", &gorm.Config{})
	if dbInstance == nil || seenDSN != "dsn" {
		t.Fatalf("mustOpenDB() = %v, seenDSN = %q", dbInstance, seenDSN)
	}
}

func TestNewRedisClientAndPing(t *testing.T) {
	server := miniredis.RunT(t)
	t.Setenv("REDIS_HOST", server.Host())
	t.Setenv("REDIS_PORT", ":"+server.Port())
	t.Setenv("REDIS_PASSWORD", "")

	client := newRedisClient(10)
	defer client.Close()

	options := client.Options()
	if options.Addr != server.Addr() || options.DB != 10 {
		t.Fatalf("redis options = %#v", options)
	}
	mustPingRedis("redis", client)
}

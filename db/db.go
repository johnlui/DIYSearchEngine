package db

import (
	"context"
	"log"
	"os"
	"time"

	"github.com/go-redis/redis/v8"
	"github.com/johnlui/enterprise-search-engine/config"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// pages 仓库数据库
var DbInstance0 *gorm.DB

// 如果你有多个数据库，可以取消注释，注册新的 DSN 信息
// var DbInstance1 *gorm.DB

// 字典数据库
var DbInstanceDic *gorm.DB

var Ctx = context.Background()
var Rdb *redis.Client
var Rdb10 *redis.Client

var openDB = mustOpenDB
var configurePool = configureDBPool
var createRedisClient = newRedisClient
var pingRedis = mustPingRedis
var makeMysqlDialector = mysql.Open
var openGorm = gorm.Open

func InitDB() {
	InitDBWithConfig(config.Load())
}

func InitDBWithConfig(cfg config.Config) {
	// 初始化 GORM

	// 如果你有多个数据库，可以取消注释，注册新的 DSN 信息
	// dsn1 := os.Getenv("DB_USERNAME1") + ":" +
	//   os.Getenv("DB_PASSWORD1") + "@(" +
	//   os.Getenv("DB_HOST1") + ":" +
	//   os.Getenv("DB_PORT1") + ")/" +
	//   os.Getenv("DB_DATABASE1") + "?charset=utf8mb4&parseTime=True&loc=Local"

	file, err := os.OpenFile("gorm-log.txt", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		log.Fatalf("open gorm log file: %v", err)
	}

	logLevel := logger.Warn
	if cfg.AppDebug {
		logLevel = logger.Info
	}
	fileLogger := logger.New(
		log.New(file, "", log.LstdFlags), // io writer（日志输出的目标，前缀和日志包含的内容——译者注）
		logger.Config{
			SlowThreshold:             time.Second * 6, // 慢 SQL 阈值
			LogLevel:                  logLevel,        // 日志级别
			IgnoreRecordNotFoundError: true,            // 忽略ErrRecordNotFound（记录未找到）错误
			Colorful:                  false,           // 禁用彩色打印
		},
	)

	gormConfig := gorm.Config{
		Logger: fileLogger,
	}

	DbInstance0 = openDB("pages", cfg.PagesDB.DSN(), &gormConfig)
	// DbInstance1 = mustOpenDB("pages-1", dsn1, &gormConfig)
	DbInstanceDic = openDB("dictionary", cfg.DictionaryDB.DSN(), &gormConfig)

	configurePool("pages", DbInstance0, cfg.PagesDB.MaxIdle, cfg.PagesDB.MaxOpen)
	configurePool("dictionary", DbInstanceDic, cfg.DictionaryDB.MaxIdle, cfg.DictionaryDB.MaxOpen)

	// 初始化 Redis
	// 默认 Redis，用作缓存
	Rdb = createRedisClientFromConfig(cfg.Redis)
	// 倒排索引字典生成中转站
	Rdb10 = createRedisClientFromConfig(cfg.IndexRedis)

	pingRedis("redis-0", Rdb)
	pingRedis("redis-10", Rdb10)
}

func mustOpenDB(name, dsn string, gormConfig *gorm.Config) *gorm.DB {
	dbInstance, err := openGorm(makeMysqlDialector(dsn), gormConfig)
	if err != nil {
		log.Fatalf("open %s db: %v", name, err)
	}
	return dbInstance
}

func configureDBPool(name string, dbInstance *gorm.DB, maxIdle, maxOpen int) {
	sqlDB, err := dbInstance.DB()
	if err != nil {
		log.Fatalf("open %s sql db: %v", name, err)
	}

	sqlDB.SetMaxIdleConns(maxIdle)
	sqlDB.SetMaxOpenConns(maxOpen)
	sqlDB.SetConnMaxLifetime(time.Hour)

	if err := sqlDB.Ping(); err != nil {
		log.Fatalf("ping %s db: %v", name, err)
	}
}

func newRedisClient(dbIndex int) *redis.Client {
	return createRedisClientFromConfig(config.Load().RedisWithDB(dbIndex))
}

func createRedisClientFromConfig(cfg config.Redis) *redis.Client {
	return redis.NewClient(&redis.Options{
		Addr:        cfg.Addr,
		Password:    cfg.Password,
		DB:          cfg.DB,
		DialTimeout: time.Second * time.Duration(cfg.DialSeconds),
		ReadTimeout: time.Second * time.Duration(cfg.ReadSeconds),
	})
}

func mustPingRedis(name string, client *redis.Client) {
	if err := client.Ping(Ctx).Err(); err != nil {
		log.Fatalf("ping %s: %v", name, err)
	}
}

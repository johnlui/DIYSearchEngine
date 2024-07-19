package db

import (
  "context"
  "log"
  "os"
  "time"

  "github.com/go-redis/redis/v8"
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

func InitDB() {
  // 初始化 GORM

  // 默认仓库数据库
  dsn0 := os.Getenv("DB_USERNAME0") + ":" +
    os.Getenv("DB_PASSWORD0") + "@(" +
    os.Getenv("DB_HOST0") + ":" +
    os.Getenv("DB_PORT0") + ")/" +
    os.Getenv("DB_DATABASE0") + "?charset=utf8mb4&parseTime=True&loc=Local"

  // 如果你有多个数据库，可以取消注释，注册新的 DSN 信息
  // dsn1 := os.Getenv("DB_USERNAME1") + ":" +
  //   os.Getenv("DB_PASSWORD1") + "@(" +
  //   os.Getenv("DB_HOST1") + ":" +
  //   os.Getenv("DB_PORT1") + ")/" +
  //   os.Getenv("DB_DATABASE1") + "?charset=utf8mb4&parseTime=True&loc=Local"

  // 字典数据库
  dsnDic := os.Getenv("DB_USERNAME_DIC") + ":" +
    os.Getenv("DB_PASSWORD_DIC") + "@(" +
    os.Getenv("DB_HOST_DIC") + ":" +
    os.Getenv("DB_PORT_DIC") + ")/" +
    os.Getenv("DB_DATABASE_DIC") + "?charset=utf8mb4&parseTime=True&loc=Local"

  // gorm SQL 日志
  file, err := os.Create("gorm-log.txt")
  if err != nil {
    panic(err)
  }

  logLevel := logger.Warn
  if os.Getenv("APP_DEBUG") == "true" {
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

  _db0, _ := gorm.Open(mysql.Open(dsn0), &gormConfig)
  // _db1, _ := gorm.Open(mysql.Open(dsn1), &gormConfig)
  _dbDic, _ := gorm.Open(mysql.Open(dsnDic), &gormConfig)

  dbdb0, _ := _db0.DB()
  dbdb0.SetMaxIdleConns(1)
  dbdb0.SetMaxOpenConns(20)
  dbdb0.SetConnMaxLifetime(time.Hour)

  // dbdb1, _ := _db1.DB()
  // dbdb1.SetMaxIdleConns(1)
  // dbdb1.SetMaxOpenConns(100)
  // dbdb1.SetConnMaxLifetime(time.Hour)

  dbdbDic, _ := _dbDic.DB()
  dbdbDic.SetMaxIdleConns(1)
  dbdbDic.SetMaxOpenConns(20)
  dbdbDic.SetConnMaxLifetime(time.Hour)

  DbInstance0 = _db0
  // DbInstance1 = _db1
  DbInstanceDic = _dbDic

  // 初始化 Redis
  // 默认 Redis，用作缓存
  Rdb = redis.NewClient(&redis.Options{
    Addr:        os.Getenv("REDIS_HOST") + os.Getenv("REDIS_PORT"),
    Password:    os.Getenv("REDIS_PASSWORD"),
    DB:          0,
    DialTimeout: time.Second,
    ReadTimeout: time.Second,
  })
  // 倒排索引字典生成中转站
  Rdb10 = redis.NewClient(&redis.Options{
    Addr:        os.Getenv("REDIS_HOST") + os.Getenv("REDIS_PORT"),
    Password:    os.Getenv("REDIS_PASSWORD"),
    DB:          10,
    DialTimeout: time.Second,
    ReadTimeout: time.Second,
  })

}

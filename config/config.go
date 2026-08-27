package config

import (
	"os"
	"strconv"
)

type MySQL struct {
	Host     string
	Port     string
	Database string
	Username string
	Password string
	MaxIdle  int
	MaxOpen  int
}

type Redis struct {
	Addr        string
	Password    string
	DB          int
	DialSeconds int
	ReadSeconds int
}

type Config struct {
	AppEnv   string
	AppDebug bool
	Port     string

	PagesDB      MySQL
	DictionaryDB MySQL
	Redis        Redis
	IndexRedis   Redis

	CrawlBatch             int
	CrawlWorkers           int
	PrepareBatch           int
	IndexPagesPerShard     int
	IndexTransferBatch     int
	IndexTransferWordDepth int64
}

func Load() Config {
	redisAddr := env("REDIS_ADDR", "")
	if redisAddr == "" {
		redisAddr = env("REDIS_HOST", "127.0.0.1") + normalizePort(env("REDIS_PORT", ":6379"))
	}

	password := os.Getenv("REDIS_PASSWORD")
	redisBase := Redis{
		Addr:        redisAddr,
		Password:    password,
		DialSeconds: intEnv("REDIS_DIAL_TIMEOUT_SECONDS", 1),
		ReadSeconds: intEnv("REDIS_READ_TIMEOUT_SECONDS", 1),
	}

	return Config{
		AppEnv:   env("APP_ENV", "local"),
		AppDebug: os.Getenv("APP_DEBUG") == "true",
		Port:     env("PORT", "10086"),

		PagesDB: MySQL{
			Host:     env("DB_HOST0", "127.0.0.1"),
			Port:     env("DB_PORT0", "3306"),
			Database: env("DB_DATABASE0", "test"),
			Username: env("DB_USERNAME0", "root"),
			Password: os.Getenv("DB_PASSWORD0"),
			MaxIdle:  intEnv("DB_MAX_IDLE_CONNS", 1),
			MaxOpen:  intEnv("DB_MAX_OPEN_CONNS", 20),
		},
		DictionaryDB: MySQL{
			Host:     env("DB_HOST_DIC", "127.0.0.1"),
			Port:     env("DB_PORT_DIC", "3306"),
			Database: env("DB_DATABASE_DIC", "test"),
			Username: env("DB_USERNAME_DIC", "root"),
			Password: os.Getenv("DB_PASSWORD_DIC"),
			MaxIdle:  intEnv("DB_DIC_MAX_IDLE_CONNS", intEnv("DB_MAX_IDLE_CONNS", 1)),
			MaxOpen:  intEnv("DB_DIC_MAX_OPEN_CONNS", intEnv("DB_MAX_OPEN_CONNS", 20)),
		},
		Redis:      redisBase,
		IndexRedis: redisWithDB(redisBase, 10),

		CrawlBatch:             intEnv("CRAWL_BATCH", 4),
		CrawlWorkers:           intEnv("CRAWL_WORKERS", 64),
		PrepareBatch:           intEnv("PREPARE_BATCH", 20),
		IndexPagesPerShard:     intEnv("INDEX_PAGES_PER_SHARD", 2),
		IndexTransferBatch:     intEnv("INDEX_TRANSFER_BATCH", 2000),
		IndexTransferWordDepth: int64(intEnv("INDEX_TRANSFER_WORD_DEPTH", 10000)),
	}
}

func (m MySQL) DSN() string {
	return m.Username + ":" + m.Password + "@(" + m.Host + ":" + m.Port + ")/" +
		m.Database + "?charset=utf8mb4&parseTime=True&loc=Local"
}

func (c Config) RedisWithDB(db int) Redis {
	return redisWithDB(c.Redis, db)
}

func env(key, fallback string) string {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	return value
}

func intEnv(key string, fallback int) int {
	raw := os.Getenv(key)
	if raw == "" {
		return fallback
	}
	value, err := strconv.Atoi(raw)
	if err != nil {
		return fallback
	}
	return value
}

func normalizePort(port string) string {
	if port == "" || port[0] == ':' {
		return port
	}
	return ":" + port
}

func redisWithDB(redis Redis, db int) Redis {
	redis.DB = db
	return redis
}

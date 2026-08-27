package config

import "testing"

func TestLoadDefaultsAndDSN(t *testing.T) {
	t.Setenv("APP_ENV", "")
	t.Setenv("APP_DEBUG", "")
	t.Setenv("REDIS_HOST", "")
	t.Setenv("REDIS_PORT", "")

	cfg := Load()
	if cfg.AppEnv != "local" || cfg.AppDebug {
		t.Fatalf("app config = %#v", cfg)
	}
	if cfg.Port != "10086" {
		t.Fatalf("port = %q", cfg.Port)
	}
	if got := cfg.PagesDB.DSN(); got != "root:@(127.0.0.1:3306)/test?charset=utf8mb4&parseTime=True&loc=Local" {
		t.Fatalf("dsn = %q", got)
	}
	if cfg.Redis.Addr != "127.0.0.1:6379" || cfg.IndexRedis.DB != 10 {
		t.Fatalf("redis config = %#v / %#v", cfg.Redis, cfg.IndexRedis)
	}
}

func TestLoadRedisAddressVariants(t *testing.T) {
	t.Setenv("REDIS_HOST", "redis")
	t.Setenv("REDIS_PORT", "6379")
	if got := Load().Redis.Addr; got != "redis:6379" {
		t.Fatalf("redis addr = %q", got)
	}

	t.Setenv("REDIS_ADDR", "cache:6380")
	if got := Load().Redis.Addr; got != "cache:6380" {
		t.Fatalf("redis addr override = %q", got)
	}
}

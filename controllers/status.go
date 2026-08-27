package controllers

import (
	"time"

	"github.com/gin-gonic/gin"
	"github.com/johnlui/enterprise-search-engine/internal/storage"
)

func SpiderStatus(c *gin.Context) {
	now := time.Now()
	values := NewSpiderStatusService().Values()

	c.HTML(200, "index.tpl", gin.H{
		"title":  "ESE状态监控面板",
		"time":   now.Format("2006-01-02 15:04:05"),
		"values": values,
	})
}

func redisIntGetter() func(string) int {
	return storage.FromGlobals().RedisInt
}

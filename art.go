package main

import (
	"fmt"

	"github.com/johnlui/enterprise-search-engine/db"
	"github.com/johnlui/enterprise-search-engine/internal/migrations"
)

type Art struct{}

func (a Art) Init() {
	if err := migrations.Run(db.DbInstance0, db.DbInstanceDic); err != nil {
		fmt.Println("数据库初始化失败:", err)
		return
	}
	fmt.Println("数据库初始化完成")
}

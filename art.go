package main

import (
	"fmt"

	"github.com/johnlui/enterprise-search-engine/db"
	"github.com/johnlui/enterprise-search-engine/tools"
)

type Art struct{}

func (a Art) Init() {

	realDB := db.DbInstance0

	// 初始化 256 张 pages 和 status 表
	for i := 0; i < 256; i++ {
		tableName := tools.HexTableName("pages", i)
		statusTableName := tools.HexTableName("status", i)

		result := realDB.Exec("CREATE TABLE `" + tableName + "` (   `id` int unsigned NOT NULL AUTO_INCREMENT,   `url` varchar(768) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci DEFAULT NULL,   `host` varchar(255) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci DEFAULT NULL,   `dic_done` tinyint DEFAULT '0',   `craw_done` tinyint NOT NULL DEFAULT '0',   `craw_time` timestamp NOT NULL DEFAULT '2001-01-01 00:00:00',   `origin_title` varchar(2000) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci DEFAULT NULL,   `referrer_id` int NOT NULL DEFAULT '0',   `scheme` varchar(255) DEFAULT NULL,   `domain1` varchar(255) DEFAULT NULL,   `domain2` varchar(255) DEFAULT NULL,   `path` varchar(2000) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci DEFAULT NULL,   `query` varchar(2000) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci DEFAULT NULL,   `title` varchar(1000) DEFAULT NULL,   `text` longtext,   `created_at` timestamp NOT NULL DEFAULT '2001-01-01 08:00:00',   PRIMARY KEY (`id`),   KEY `url` (`url`),   KEY `host_crtime` (`host`),   KEY `host_cdown` (`host`) ) ENGINE=InnoDB AUTO_INCREMENT=1 DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;")
		fmt.Println("建表结果: ", tableName, result.RowsAffected)

		result1 := realDB.Exec("CREATE TABLE `" + statusTableName + "` (   `id` int unsigned NOT NULL AUTO_INCREMENT,   `url` varchar(767) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci DEFAULT NULL,   `host` varchar(255) DEFAULT NULL,   `craw_done` tinyint NOT NULL DEFAULT '0',   `craw_time` timestamp NOT NULL DEFAULT '2001-01-01 00:00:00',   PRIMARY KEY (`id`),   KEY `idx_host_crtime` (`host`,`craw_time`),   KEY `idx_url` (`url`) ) ENGINE=InnoDB AUTO_INCREMENT=1 DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;")
		fmt.Println("建表结果: ", statusTableName, result1.RowsAffected)

	}
	// 初始化域名黑名单
	result2 := realDB.Exec("CREATE TABLE `domain_black_list` (   `id` int unsigned NOT NULL AUTO_INCREMENT,   `domain` varchar(255) DEFAULT NULL,   PRIMARY KEY (`id`) ) ENGINE=InnoDB AUTO_INCREMENT=1 DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;")
	fmt.Println("建表结果: ", "domain_black_list", result2.RowsAffected)
	// 填充域名黑名单
	result3 := realDB.Exec("INSERT INTO `domain_black_list` (`id`, `domain`) VALUES   (1, 'huangye88.com'),   (2, 'gov.cn'),   (3, 'nbhesen.com'),   (4, 'tianyancha.com'),   (5, 'qianlima.com'),   (6, '99114.com'),   (7, 'luosi.com'),   (8, 'bidchance.com'),   (9, '51zhantai.com'),   (10, 'baiye5.com'),   (11, 'snxx.com'),   (12, '6789go.com'),   (13, 'gongxiangchi.com'),   (14, 'webacg.com'),   (16, '912688.com'),   (17, 'dihe.cn'),   (18, 'maoyihang.com'),   (19, 'realsee.com'),   (20, 'tdzyw.com'),   (21, 'anjuke.com'),   (22, 'liuxue86.com'),   (23, '5588.tv'),   (24, '58.com');")
	fmt.Println("填充结果: ", "domain_black_list", result3.RowsAffected)

	// 初始化字典词黑名单
	result4 := realDB.Exec("CREATE TABLE `word_black_list` (   `id` int unsigned NOT NULL AUTO_INCREMENT,   `word` varchar(255) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci DEFAULT NULL,   PRIMARY KEY (`id`) ) ENGINE=InnoDB AUTO_INCREMENT=1 DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;")
	fmt.Println("建表结果: ", "word_black_list", result4.RowsAffected)
	// 填充字典词黑名单
	result5 := realDB.Exec("INSERT INTO `word_black_list` (`id`, `word`) VALUES   (1, 'px'),   (2, '20'),   (3, '('),   (4, ')'),   (5, ','),   (6, '.'),   (7, '-'),   (8, '/'),   (9, ':'),   (10, 'var'),   (11, '的'),   (12, 'com'),   (13, ';'),   (14, '['),   (15, ']'),   (16, '{'),   (17, '}'),   (18, \"'\"),   (19, '\"'),   (20, '_'),   (21, '?'),   (22, 'function'),   (23, 'document'),   (24, '|'),   (25, '='),   (26, 'html'),   (27, '内容'),   (28, '0'),   (29, '1'),   (30, '3'),   (31, 'https'),   (32, 'http'),   (33, '2'),   (34, '!'),   (35, 'window'),   (36, 'if'),   (37, '“'),   (38, '”'),   (39, '。'),   (40, 'src'),   (41, '中'),   (42, '了'),   (43, '6'),   (44, '｡'),   (45, '<'),   (46, '>'),   (47, '联系'),   (48, '号'),   (49, 'getElementsByTagName'),   (50, '5'),   (51, '､'),   (52, 'script'),   (53, 'js');")
	fmt.Println("填充结果: ", "word_black_list", result5.RowsAffected)

	// 初始化 kvstores
	result6 := realDB.Exec("CREATE TABLE `kvstores` (   `id` int unsigned NOT NULL AUTO_INCREMENT,   `k` varchar(255) DEFAULT NULL,   `v` varchar(255) DEFAULT NULL,   `time` timestamp NOT NULL DEFAULT '2001-01-01 00:00:01',   PRIMARY KEY (`id`) ) ENGINE=InnoDB AUTO_INCREMENT=1 DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;")
	fmt.Println("建表结果: ", "kvstores", result6.RowsAffected)
	// 填充 kvstores
	result7 := realDB.Exec("INSERT INTO `kvstores` (`id`, `k`, `v`, `time`) VALUES   (1, 'stop', '0', '2022-09-04 01:27:55'),   (2, 'stopNew', '0', '2001-01-01 00:00:01'),   (3, 'stopWashDicRedisToMySQL', '0', '2001-01-01 00:00:01');")
	fmt.Println("填充结果: ", "kvstores", result7.RowsAffected)

	// 初始化 字典表 word_dics
	result8 := db.DbInstanceDic.Exec("CREATE TABLE `word_dics` (   `id` int unsigned NOT NULL AUTO_INCREMENT,   `name` varchar(255) DEFAULT NULL,   `positions` longtext CHARACTER SET utf8mb4 COLLATE utf8mb4_bin,   PRIMARY KEY (`id`),   UNIQUE KEY `name` (`name`) ) ENGINE=InnoDB AUTO_INCREMENT=1 DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci")
	fmt.Println("建表结果: ", "word_dics", result8.RowsAffected)

	fmt.Println("数据库初始化完成")
}

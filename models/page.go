package models

import (
  "time"
)

type Page struct {
  ID          uint      `gorm:"primaryKey"`
  Url         string    `gorm:"default:null"`
  Host        string    `gorm:"default:null"`
  CrawDone    int       `gorm:"type:tinyint(1);default:0"`
  DicDone     int       `gorm:"type:tinyint(1);default:0"`
  CrawTime    time.Time `gorm:"default:'2001-01-01 00:00:01'"`
  OriginTitle string    `gorm:"default:null"`
  ReferrerId  uint      `gorm:"default:0"`
  Scheme      string    `gorm:"default:null"`
  Domain1     string    `gorm:"default:null"`
  Domain2     string    `gorm:"default:null"`
  Path        string    `gorm:"default:null"`
  Query       string    `gorm:"default:null"`
  Title       string    `gorm:"default:null"`
  Text        string    `gorm:"default:null"`
  CreatedAt   time.Time
}

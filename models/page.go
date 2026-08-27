package models

import (
	"time"
)

type Page struct {
	ID          uint      `gorm:"primaryKey"`
	Url         string    `gorm:"size:768;default:null;index"`
	Host        string    `gorm:"size:255;default:null;index"`
	CrawDone    int       `gorm:"type:tinyint(1);default:0"`
	DicDone     int       `gorm:"type:tinyint(1);default:0;index"`
	CrawTime    time.Time `gorm:"default:'2001-01-01 00:00:01'"`
	OriginTitle string    `gorm:"size:2000;default:null"`
	ReferrerId  uint      `gorm:"default:0"`
	Scheme      string    `gorm:"size:255;default:null"`
	Domain1     string    `gorm:"size:255;default:null"`
	Domain2     string    `gorm:"size:255;default:null"`
	Path        string    `gorm:"size:2000;default:null"`
	Query       string    `gorm:"size:2000;default:null"`
	Title       string    `gorm:"size:1000;default:null"`
	Text        string    `gorm:"type:longtext"`
	CreatedAt   time.Time
}

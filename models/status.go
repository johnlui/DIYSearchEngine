package models

import (
	"time"
)

type Status struct {
	ID       uint      `gorm:"primaryKey"`
	Url      string    `gorm:"size:767;default:null;index"`
	Host     string    `gorm:"size:255;default:null;index"`
	CrawDone int       `gorm:"type:tinyint(1);default:0"`
	CrawTime time.Time `gorm:"default:'2001-01-01 00:00:01';index"`
}

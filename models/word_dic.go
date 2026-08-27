package models

type WordDic struct {
	ID        uint   `gorm:"primaryKey"`
	Name      string `gorm:"size:255;default:null;uniqueIndex"`
	Positions string `gorm:"type:longtext"`
}

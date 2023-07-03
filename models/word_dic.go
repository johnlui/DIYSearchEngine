package models

type WordDic struct {
  ID        uint   `gorm:"primaryKey"`
  Name      string `gorm:"default:null"`
  Positions string `gorm:"default:null"`
}

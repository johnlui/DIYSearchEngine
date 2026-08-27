package models

type WordPosting struct {
	ID            uint   `gorm:"primaryKey"`
	Term          string `gorm:"uniqueIndex:uidx_term_doc,priority:1;index;size:255"`
	TableIndex    int    `gorm:"uniqueIndex:uidx_term_doc,priority:2"`
	DocID         uint   `gorm:"uniqueIndex:uidx_term_doc,priority:3"`
	TermFrequency int
	DocLength     int
	Positions     string `gorm:"type:text"`
}

package indexstore

import (
	"strconv"
	"strings"

	"github.com/johnlui/enterprise-search-engine/models"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const WordPostingsTable = "word_postings"

func ParsePostings(term, payload string) []models.WordPosting {
	if term == "" || payload == "" {
		return nil
	}

	postings := make([]models.WordPosting, 0)
	for _, rawPart := range strings.Split(payload, "-") {
		if rawPart == "" {
			continue
		}

		fields := strings.Split(rawPart, ",")
		if len(fields) < 4 {
			continue
		}

		tableIndex, err := strconv.Atoi(fields[0])
		if err != nil {
			continue
		}

		docID, err := strconv.ParseUint(fields[1], 10, 64)
		if err != nil {
			continue
		}

		termFrequency, err := strconv.Atoi(fields[2])
		if err != nil {
			continue
		}

		docLength, err := strconv.Atoi(fields[3])
		if err != nil {
			continue
		}

		posting := models.WordPosting{
			Term:          term,
			TableIndex:    tableIndex,
			DocID:         uint(docID),
			TermFrequency: termFrequency,
			DocLength:     docLength,
		}
		if len(fields) > 4 {
			posting.Positions = strings.Join(fields[4:], ",")
		}

		postings = append(postings, posting)
	}

	return postings
}

func AppendPostings(tx *gorm.DB, term, payload string) error {
	postings := ParsePostings(term, payload)
	if len(postings) == 0 {
		return nil
	}

	return tx.Table(WordPostingsTable).
		Clauses(clause.OnConflict{
			Columns: []clause.Column{
				{Name: "term"},
				{Name: "table_index"},
				{Name: "doc_id"},
			},
			DoUpdates: clause.AssignmentColumns([]string{"term_frequency", "doc_length", "positions"}),
		}).
		CreateInBatches(postings, 500).Error
}

func EnsureWordDic(db *gorm.DB, term string) error {
	if term == "" {
		return nil
	}
	return db.Table("word_dics").
		Clauses(clause.OnConflict{DoNothing: true}).
		Create(&models.WordDic{Name: term, Positions: ""}).Error
}

func AppendLegacyWordDic(tx *gorm.DB, term, payload string) error {
	if term == "" || payload == "" {
		return nil
	}

	expr := gorm.Expr("concat(ifnull(positions,''), ?)", payload)
	if tx.Dialector.Name() == "sqlite" {
		expr = gorm.Expr("COALESCE(positions,'') || ?", payload)
	}

	return tx.Table("word_dics").Where("name = ?", term).Update("positions", expr).Error
}

func LoadPostings(db *gorm.DB, terms []string) (map[string][]models.WordPosting, error) {
	result := make(map[string][]models.WordPosting, len(terms))
	if len(terms) == 0 {
		return result, nil
	}
	if !db.Migrator().HasTable(WordPostingsTable) {
		return result, nil
	}

	var postings []models.WordPosting
	if err := db.Table(WordPostingsTable).Where("term IN ?", terms).Find(&postings).Error; err != nil {
		return result, err
	}

	for _, posting := range postings {
		result[posting.Term] = append(result[posting.Term], posting)
	}
	return result, nil
}

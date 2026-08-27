package storage

import (
	"fmt"

	"github.com/johnlui/enterprise-search-engine/internal/indexstore"
	"github.com/johnlui/enterprise-search-engine/models"
)

func (h Handles) LoadWordDics(words []string) map[string]models.WordDic {
	if len(words) == 0 {
		return map[string]models.WordDic{}
	}

	dics := make([]models.WordDic, 0, len(words))
	h.DictionaryDB.Where("name IN ?", words).Find(&dics)

	result := make(map[string]models.WordDic, len(dics))
	for _, dic := range dics {
		result[dic.Name] = dic
	}
	return result
}

func (h Handles) LoadPostings(words []string) map[string][]models.WordPosting {
	postingsByWord, err := indexstore.LoadPostings(h.DictionaryDB, words)
	if err != nil {
		return map[string][]models.WordPosting{}
	}
	return postingsByWord
}

func (h Handles) LoadPagesByTableIDs(idsByTable map[int][]uint) map[string]models.Page {
	pagesByDocKey := make(map[string]models.Page)
	for tableIndex, ids := range idsByTable {
		tableName := h.PageTable(tableIndex)
		var pages []models.Page
		h.PagesDB.Table(tableName).Where("id IN ?", ids).Find(&pages)

		for _, page := range pages {
			docKey := fmt.Sprintf("%d-%d", tableIndex, page.ID)
			pagesByDocKey[docKey] = page
		}
	}
	return pagesByDocKey
}

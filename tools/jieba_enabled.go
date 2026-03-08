//go:build cgo

package tools

import (
	"path/filepath"

	"github.com/yanyiwu/gojieba"
)

var jiebaInstance *gojieba.Jieba

func InitJieba(dictDir string) {
	jiebaPath := filepath.Join(dictDir, "jieba.dict.utf8")
	hmmPath := filepath.Join(dictDir, "hmm_model.utf8")
	userPath := filepath.Join(dictDir, "user.dict.utf8")
	idfPath := filepath.Join(dictDir, "idf.utf8")
	stopPath := filepath.Join(dictDir, "stop_words.utf8")

	jiebaInstance = gojieba.NewJieba(jiebaPath, hmmPath, userPath, idfPath, stopPath)
	jiebaCut = func(s string) []string {
		if s == "" {
			return nil
		}
		return jiebaInstance.CutForSearch(s, true)
	}
}

package tools

import (
	"crypto/md5"
	"encoding/hex"
	"net/url"
	"regexp"
	"unicode"

	"github.com/yanyiwu/gojieba"
)

var JiebaInstance *gojieba.Jieba

// 生成小写MD5哈希值
func GetMD5Hash(text string) string {
	hash := md5.Sum([]byte(text))
	return hex.EncodeToString(hash[:])
}

// 是否是合法URL
func IsUrl(str string) bool {
	u, err := url.Parse(str)
	return err == nil && u.Scheme != "" && u.Host != ""
}

// 去除所有的空格和换行
func StringStrip(input string) string {
	if input == "" {
		return ""
	}
	reg := regexp.MustCompile(`[\s\p{Zs}]{1,}`)
	return reg.ReplaceAllString(input, "-")
}

// 首字母大写
func FirstLetterUppercase(input string) string {
	r := []rune(input)
	return string(append([]rune{unicode.ToUpper(r[0])}, r[1:]...))
}

// 结巴分词
func GetFenciResultArray(s string) []string {
	result := []string{}
	result = JiebaInstance.CutForSearch(s, true)
	return result
}

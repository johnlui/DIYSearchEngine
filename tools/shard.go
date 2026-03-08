package tools

import (
	"fmt"
	"strconv"
	"time"
)

func HexTableName(prefix string, index int) string {
	return fmt.Sprintf("%s_%02x", prefix, index)
}

func MD5TableName(prefix, value string) string {
	return prefix + "_" + GetMD5Hash(value)[0:2]
}

func MinuteBucketKey(prefix string, now time.Time) string {
	return prefix + strconv.FormatInt(now.Unix()/60, 10)
}

func WindowBucketKey(prefix, host string, windowSeconds int, now time.Time) string {
	return prefix + host + "_" + strconv.Itoa(windowSeconds) + "s_" + strconv.FormatInt(now.Unix()/int64(windowSeconds), 10)
}

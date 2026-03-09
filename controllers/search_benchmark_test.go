package controllers

import (
	"strconv"
	"strings"
	"testing"
)

func BenchmarkParseBestDocParts(b *testing.B) {
	var builder strings.Builder
	builder.WriteString("-")
	for i := 0; i < 500; i++ {
		builder.WriteString("1,")
		builder.WriteString(strconv.Itoa(i))
		builder.WriteString(",")
		builder.WriteString(strconv.Itoa(i%7 + 1))
		builder.WriteString(",120,0-")
	}
	positions := builder.String()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = parseBestDocParts(positions)
	}
}

func BenchmarkTopDocKeysByScore(b *testing.B) {
	scores := make(map[string]float64, 5000)
	for i := 0; i < 5000; i++ {
		scores[strconv.Itoa(i)] = float64(5000 - i)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = topDocKeysByScore(scores, 200)
	}
}

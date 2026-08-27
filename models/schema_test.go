package models

import (
	"reflect"
	"strings"
	"testing"
)

func TestSearchSchemaKeepsLegacyColumnCapacity(t *testing.T) {
	cases := []struct {
		model     any
		fieldName string
		wantParts []string
	}{
		{Page{}, "Url", []string{"size:768", "index"}},
		{Page{}, "OriginTitle", []string{"size:2000"}},
		{Page{}, "Path", []string{"size:2000"}},
		{Page{}, "Query", []string{"size:2000"}},
		{Page{}, "Title", []string{"size:1000"}},
		{Page{}, "Text", []string{"type:longtext"}},
		{Status{}, "Url", []string{"size:767", "index"}},
		{WordDic{}, "Name", []string{"size:255", "uniqueIndex"}},
		{WordDic{}, "Positions", []string{"type:longtext"}},
	}

	for _, tc := range cases {
		field, ok := reflect.TypeOf(tc.model).FieldByName(tc.fieldName)
		if !ok {
			t.Fatalf("%T.%s not found", tc.model, tc.fieldName)
		}

		tag := field.Tag.Get("gorm")
		for _, wantPart := range tc.wantParts {
			if !strings.Contains(tag, wantPart) {
				t.Fatalf("%T.%s gorm tag = %q, missing %q", tc.model, tc.fieldName, tag, wantPart)
			}
		}
	}
}

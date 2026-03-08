package tools

import "testing"

func TestGetMD5Hash(t *testing.T) {
	if got := GetMD5Hash("hello"); got != "5d41402abc4b2a76b9719d911017c592" {
		t.Fatalf("GetMD5Hash() = %q", got)
	}
}

func TestIsURL(t *testing.T) {
	if !IsUrl("https://example.com/path?q=1") {
		t.Fatal("expected valid URL")
	}
	if IsUrl("example.com/path") {
		t.Fatal("expected invalid URL without scheme")
	}
}

func TestStringStrip(t *testing.T) {
	if got := StringStrip("a \n\t b　c"); got != "a-b-c" {
		t.Fatalf("StringStrip() = %q", got)
	}
	if got := StringStrip(""); got != "" {
		t.Fatalf("StringStrip(empty) = %q", got)
	}
}

func TestFirstLetterUppercase(t *testing.T) {
	if got := FirstLetterUppercase("hello"); got != "Hello" {
		t.Fatalf("FirstLetterUppercase() = %q", got)
	}
	if got := FirstLetterUppercase(""); got != "" {
		t.Fatalf("FirstLetterUppercase(empty) = %q", got)
	}
}

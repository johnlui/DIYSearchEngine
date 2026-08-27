package tools

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/johnlui/enterprise-search-engine/models"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}

type errReader struct{}

func (errReader) Read([]byte) (int, error) { return 0, errors.New("read failed") }
func (errReader) Close() error             { return nil }

func TestFallbackCutForSearch(t *testing.T) {
	got := fallbackCutForSearch("Hello,中文 123")
	want := []string{"hello", "中", "文", "中文", "123"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("fallbackCutForSearch() = %#v, want %#v", got, want)
	}

	if got := fallbackCutForSearch(""); got != nil {
		t.Fatalf("fallbackCutForSearch(empty) = %#v, want nil", got)
	}
}

func TestGetFenciResultArrayUsesConfiguredCutter(t *testing.T) {
	original := jiebaCut
	defer func() { jiebaCut = original }()

	jiebaCut = func(s string) []string {
		return []string{s, "configured"}
	}

	got := GetFenciResultArray("keyword")
	want := []string{"keyword", "configured"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("GetFenciResultArray() = %#v, want %#v", got, want)
	}
}

func TestIsASCIIWord(t *testing.T) {
	if !isASCIIWord("abc123") {
		t.Fatal("expected ASCII letters and digits to be accepted")
	}
	if isASCIIWord("abc-123") {
		t.Fatal("expected punctuation to be rejected")
	}
	if isASCIIWord("中文") {
		t.Fatal("expected non-ASCII runes to be rejected")
	}
}

func TestInitJieba(t *testing.T) {
	original := jiebaCut
	defer func() { jiebaCut = original }()

	dictDir := filepath.Join("..", "dict")
	InitJieba(dictDir)

	if got := GetFenciResultArray("中文搜索"); len(got) == 0 {
		t.Fatal("expected jieba to return search tokens")
	}
}

func TestAddDouhao(t *testing.T) {
	if got := AddDouhao(1234567890); got != "1,234,567,890" {
		t.Fatalf("AddDouhao() = %q", got)
	}
}

func TestDDUsesExitHook(t *testing.T) {
	original := exit
	defer func() { exit = original }()

	code := -1
	exit = func(v int) {
		code = v
	}

	DD("debug")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
}

func TestCurlSuccessAndReadError(t *testing.T) {
	original := client
	defer func() { client = original }()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("User-Agent"); got == "" {
			t.Fatal("expected user agent header")
		}
		w.WriteHeader(http.StatusFound)
		_, _ = w.Write([]byte("<html><head><title>ok</title></head><body>body</body></html>"))
	}))
	defer server.Close()

	client = server.Client()
	client.CheckRedirect = original.CheckRedirect

	doc, code := Curl(models.Status{Url: server.URL})
	if code != 1 {
		t.Fatalf("Curl() code = %d, want 1", code)
	}
	if title := doc.Find("title").Text(); title != "ok" {
		t.Fatalf("Curl() title = %q", title)
	}

	client = &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Body: errReader{}}, nil
	})}
	_, code = Curl(models.Status{Url: "https://example.com"})
	if code != 3 {
		t.Fatalf("Curl(read error) code = %d, want 3", code)
	}
}

func TestCurlTransportError(t *testing.T) {
	originalClient := client
	originalFailure := countCurlFailure
	defer func() {
		client = originalClient
		countCurlFailure = originalFailure
	}()

	client = &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return nil, io.ErrUnexpectedEOF
	})}
	countCurlFailure = func(status models.Status) int {
		if status.Url != "https://example.com" {
			t.Fatalf("status.Url = %q", status.Url)
		}
		return 2
	}

	_, code := Curl(models.Status{Url: "https://example.com"})
	if code != 2 {
		t.Fatalf("Curl(transport error) code = %d, want 2", code)
	}
}

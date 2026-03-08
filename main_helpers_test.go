package main

import "testing"

func TestRunArtCommand(t *testing.T) {
	called := false
	ok := runArtCommand(map[string]artCommand{
		"init": func(args ...string) {
			called = len(args) == 2 && args[0] == "a" && args[1] == "b"
		},
	}, []string{"init", "a", "b"})
	if !ok || !called {
		t.Fatal("expected command to run")
	}
}

func TestRunArtCommandMissing(t *testing.T) {
	if runArtCommand(map[string]artCommand{}, []string{"missing"}) {
		t.Fatal("expected missing command to return false")
	}
	if runArtCommand(map[string]artCommand{}, nil) {
		t.Fatal("expected empty args to return false")
	}
}

func TestCollectCrawlResults(t *testing.T) {
	chs := []chan int{make(chan int, 1), make(chan int, 1), make(chan int, 1)}
	chs[0] <- 1
	chs[1] <- 2
	chs[2] <- 1

	got := collectCrawlResults(chs)
	if got[1] != 2 || got[2] != 1 {
		t.Fatalf("collectCrawlResults() = %#v", got)
	}
}

package crawlqueue

import (
	"context"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/go-redis/redis/v8"
	"github.com/johnlui/enterprise-search-engine/internal/keys"
	"github.com/johnlui/enterprise-search-engine/models"
)

func TestQueuePushAndPopBatch(t *testing.T) {
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() {
		client.Close()
	})

	queue := New(client, context.Background())
	if queue.Key != keys.CrawlQueue {
		t.Fatalf("queue key = %q, want %q", queue.Key, keys.CrawlQueue)
	}
	if err := queue.Push(nil); err != nil {
		t.Fatalf("empty Push() = %v", err)
	}
	if got := queue.PopBatch(0); got != nil {
		t.Fatalf("PopBatch(0) = %#v", got)
	}

	statuses := []models.Status{
		{ID: 1, Url: "https://one.example", Host: "one.example"},
		{ID: 2, Url: "https://two.example", Host: "two.example"},
	}
	if err := queue.Push(statuses); err != nil {
		t.Fatalf("Push() = %v", err)
	}
	client.LPush(context.Background(), queue.Key, "{bad-json")

	popped := queue.PopBatch(3)
	if len(popped) != 2 {
		t.Fatalf("PopBatch() len = %d, want 2: %#v", len(popped), popped)
	}
	if popped[0].ID != 1 || popped[1].ID != 2 {
		t.Fatalf("PopBatch() = %#v", popped)
	}
	if empty := queue.PopBatch(1); len(empty) != 0 {
		t.Fatalf("empty queue pop = %#v", empty)
	}
}

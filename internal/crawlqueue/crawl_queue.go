package crawlqueue

import (
	"context"
	"encoding/json"

	"github.com/go-redis/redis/v8"
	"github.com/johnlui/enterprise-search-engine/internal/keys"
	"github.com/johnlui/enterprise-search-engine/models"
)

type Queue struct {
	Client *redis.Client
	Ctx    context.Context
	Key    string
}

func New(client *redis.Client, ctx context.Context) Queue {
	return Queue{
		Client: client,
		Ctx:    ctx,
		Key:    keys.CrawlQueue,
	}
}

func (q Queue) Push(statuses []models.Status) error {
	if len(statuses) == 0 {
		return nil
	}

	payloads := make([]any, 0, len(statuses))
	for _, status := range statuses {
		payload, err := json.Marshal(status)
		if err != nil {
			return err
		}
		payloads = append(payloads, payload)
	}

	return q.Client.LPush(q.Ctx, q.Key, payloads...).Err()
}

func (q Queue) PopBatch(count int) []models.Status {
	if count <= 0 {
		return nil
	}

	pipe := q.Client.Pipeline()
	cmds := make([]*redis.StringCmd, count)
	for i := 0; i < count; i++ {
		cmds[i] = pipe.RPop(q.Ctx, q.Key)
	}
	_, _ = pipe.Exec(q.Ctx)

	statuses := make([]models.Status, 0, count)
	for _, cmd := range cmds {
		jsonString, err := cmd.Result()
		if err != nil {
			continue
		}

		var status models.Status
		if err := json.Unmarshal([]byte(jsonString), &status); err != nil {
			continue
		}
		statuses = append(statuses, status)
	}

	return statuses
}

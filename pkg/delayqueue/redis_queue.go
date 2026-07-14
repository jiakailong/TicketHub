package delayqueue

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

const defaultVisibilityTimeout = 2 * time.Minute

var enqueueScript = redis.NewScript(`
redis.call("HSET", KEYS[2], ARGV[1], ARGV[2])
redis.call("ZADD", KEYS[1], ARGV[3], ARGV[1])
redis.call("ZREM", KEYS[3], ARGV[1])
return 1
`)

var claimScript = redis.NewScript(`
local now = tonumber(ARGV[1])
local visibility_deadline = tonumber(ARGV[2])
local limit = tonumber(ARGV[3])

local expired = redis.call("ZRANGEBYSCORE", KEYS[2], "-inf", now, "LIMIT", 0, limit)
for _, id in ipairs(expired) do
  if redis.call("ZREM", KEYS[2], id) == 1 then
    redis.call("ZADD", KEYS[1], now, id)
  end
end

local legacy = redis.call("ZRANGEBYSCORE", KEYS[4], "-inf", now, "LIMIT", 0, limit)
for _, raw in ipairs(legacy) do
  local ok, decoded = pcall(cjson.decode, raw)
  if ok then
    local id = tostring(decoded.ID or decoded.id)
    if id ~= "nil" and redis.call("ZREM", KEYS[4], raw) == 1 then
      redis.call("HSET", KEYS[3], id, raw)
      redis.call("ZADD", KEYS[1], now, id)
    end
  end
end

local ids = redis.call("ZRANGEBYSCORE", KEYS[1], "-inf", now, "LIMIT", 0, limit)
local claimed = {}
for _, id in ipairs(ids) do
  if redis.call("ZREM", KEYS[1], id) == 1 then
    redis.call("ZADD", KEYS[2], visibility_deadline, id)
    table.insert(claimed, id)
  end
end
return claimed
`)

var ackScript = redis.NewScript(`
redis.call("ZREM", KEYS[1], ARGV[1])
redis.call("ZREM", KEYS[2], ARGV[1])
redis.call("HDEL", KEYS[3], ARGV[1])
return 1
`)

type RedisQueue struct {
	client            redis.Cmdable
	prefix            string
	visibilityTimeout time.Duration
}

func NewRedisQueue(client redis.Cmdable, prefix string) RedisQueue {
	if prefix == "" {
		prefix = "tickethub:delayqueue"
	}
	return RedisQueue{client: client, prefix: prefix, visibilityTimeout: defaultVisibilityTimeout}
}

func (q RedisQueue) WithVisibilityTimeout(timeout time.Duration) RedisQueue {
	if timeout > 0 {
		q.visibilityTimeout = timeout
	}
	return q
}

func (q RedisQueue) Enqueue(ctx context.Context, msg Message) error {
	if msg.ID == "" || msg.Topic == "" {
		return fmt.Errorf("delay queue message id and topic are required")
	}
	data, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	keys := q.keys(msg.Topic)
	return enqueueScript.Run(ctx, q.client, []string{keys.ready, keys.data, keys.processing}, msg.ID, data, msg.AvailableAt.UnixMilli()).Err()
}

func (q RedisQueue) ClaimDue(ctx context.Context, topic string, now time.Time, limit int) ([]Message, error) {
	if limit <= 0 {
		return nil, nil
	}
	keys := q.keys(topic)
	visibility := q.visibilityTimeout
	if visibility <= 0 {
		visibility = defaultVisibilityTimeout
	}
	claimed, err := claimScript.Run(ctx, q.client, []string{keys.ready, keys.processing, keys.data, keys.legacy}, now.UnixMilli(), now.Add(visibility).UnixMilli(), limit).StringSlice()
	if err != nil {
		return nil, err
	}
	if len(claimed) == 0 {
		return nil, nil
	}
	values, err := q.client.HMGet(ctx, keys.data, claimed...).Result()
	if err != nil {
		return nil, err
	}
	messages := make([]Message, 0, len(claimed))
	for index, value := range values {
		if value == nil {
			_ = q.Ack(ctx, topic, claimed[index])
			continue
		}
		var msg Message
		if err := json.Unmarshal([]byte(fmt.Sprint(value)), &msg); err != nil {
			return nil, err
		}
		msg.Attempts++
		updated, err := json.Marshal(msg)
		if err != nil {
			return nil, err
		}
		if err := q.client.HSet(ctx, keys.data, msg.ID, updated).Err(); err != nil {
			return nil, err
		}
		messages = append(messages, msg)
	}
	return messages, nil
}

func (q RedisQueue) Ack(ctx context.Context, topic string, id string) error {
	keys := q.keys(topic)
	return ackScript.Run(ctx, q.client, []string{keys.ready, keys.processing, keys.data}, id).Err()
}

type queueKeys struct {
	ready      string
	processing string
	data       string
	legacy     string
}

func (q RedisQueue) keys(topic string) queueKeys {
	base := fmt.Sprintf("%s:%s", q.prefix, topic)
	return queueKeys{ready: base + ":ready", processing: base + ":processing", data: base + ":data", legacy: base}
}

// Package keywalk holds the two ways to ask a single-threaded server for a set of keys, because the
// difference between them is not speed - it is whether every other client is served while the walk
// happens.
package keywalk

import (
	"context"

	"github.com/redis/go-redis/v9"
)

// Keys is the call that has taken down game servers: O(N) on a single-threaded server, holding the
// whole reply in memory on both sides. It is correct and it is here so that the lab can measure it.
func Keys(ctx context.Context, c *redis.Client, pattern string) ([]string, error) {
	return c.Keys(ctx, pattern).Result()
}

// Scan walks the keyspace in increments, so the server serves other clients between batches. It is
// not faster in wall-clock terms - it is often slower - and that is the trade: latency for others
// instead of a stall for everyone.
func Scan(ctx context.Context, c *redis.Client, pattern string, count int64) ([]string, error) {
	if count <= 0 {
		count = 1000
	}
	var (
		cursor uint64
		out    []string
	)
	for {
		keys, next, err := c.Scan(ctx, cursor, pattern, count).Result()
		if err != nil {
			return nil, err
		}
		out = append(out, keys...)
		cursor = next
		if cursor == 0 {
			return out, nil
		}
	}
}

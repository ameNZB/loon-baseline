// Package redis is the Redis-backed cache.Cache impl (go-redis v9). A host binds
// it in place of cache/memory when a shared cache is needed across processes;
// no call site changes.
package redis

import (
	"context"
	"errors"
	"time"

	goredis "github.com/redis/go-redis/v9"

	"github.com/ameNZB/loon-baseline/cache"
)

// Cache wraps a go-redis client as a cache.Cache.
type Cache struct{ rdb *goredis.Client }

// New builds a Redis-backed cache over an existing client.
func New(rdb *goredis.Client) *Cache { return &Cache{rdb: rdb} }

var _ cache.Cache = (*Cache)(nil)

func (c *Cache) Get(ctx context.Context, key string) ([]byte, bool, error) {
	b, err := c.rdb.Get(ctx, key).Bytes()
	if errors.Is(err, goredis.Nil) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	return b, true, nil
}

func (c *Cache) Set(ctx context.Context, key string, val []byte, ttl time.Duration) error {
	return c.rdb.Set(ctx, key, val, ttl).Err()
}

func (c *Cache) Delete(ctx context.Context, key string) error {
	return c.rdb.Del(ctx, key).Err()
}

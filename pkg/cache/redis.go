package cache

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/go-redis/redis/v8"
	"github.com/hypasis/sync-protocol/internal/types"
)

// RedisCache implements a distributed cache using Redis
type RedisCache struct {
	client *redis.Client
	config *RedisConfig
	ctx    context.Context
}

// RedisConfig configures the Redis cache
type RedisConfig struct {
	URL         string        // Redis URL (redis://host:port)
	Password    string        // Redis password
	DB          int           // Redis database number
	MaxRetries  int           // Maximum number of retries
	PoolSize    int           // Connection pool size
	MinIdleConn int           // Minimum idle connections
	TTL         time.Duration // Default TTL for cached items
	Prefix      string        // Key prefix for namespacing
}

// NewRedisCache creates a new Redis cache
func NewRedisCache(cfg *RedisConfig) (*RedisCache, error) {
	opts, err := redis.ParseURL(cfg.URL)
	if err != nil {
		return nil, fmt.Errorf("invalid Redis URL: %w", err)
	}

	// Override with config
	if cfg.Password != "" {
		opts.Password = cfg.Password
	}
	if cfg.DB > 0 {
		opts.DB = cfg.DB
	}
	if cfg.MaxRetries > 0 {
		opts.MaxRetries = cfg.MaxRetries
	}
	if cfg.PoolSize > 0 {
		opts.PoolSize = cfg.PoolSize
	}
	if cfg.MinIdleConn > 0 {
		opts.MinIdleConns = cfg.MinIdleConn
	}

	client := redis.NewClient(opts)
	ctx := context.Background()

	// Test connection
	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("failed to connect to Redis: %w", err)
	}

	fmt.Printf("✅ Connected to Redis at %s\n", cfg.URL)

	return &RedisCache{
		client: client,
		config: cfg,
		ctx:    ctx,
	}, nil
}

// Close closes the Redis connection
func (c *RedisCache) Close() error {
	return c.client.Close()
}

// key generates a namespaced key
func (c *RedisCache) key(k string) string {
	if c.config.Prefix != "" {
		return fmt.Sprintf("%s:%s", c.config.Prefix, k)
	}
	return k
}

// GetBlock retrieves a block from cache
func (c *RedisCache) GetBlock(ctx context.Context, blockNum uint64) (*types.Block, error) {
	key := c.key(fmt.Sprintf("block:%d", blockNum))

	data, err := c.client.Get(ctx, key).Bytes()
	if err == redis.Nil {
		return nil, fmt.Errorf("block not in cache")
	}
	if err != nil {
		return nil, fmt.Errorf("cache error: %w", err)
	}

	var block types.Block
	if err := json.Unmarshal(data, &block); err != nil {
		return nil, fmt.Errorf("failed to unmarshal block: %w", err)
	}

	return &block, nil
}

// SetBlock stores a block in cache
func (c *RedisCache) SetBlock(ctx context.Context, block *types.Block) error {
	key := c.key(fmt.Sprintf("block:%d", block.Header.Number))

	data, err := json.Marshal(block)
	if err != nil {
		return fmt.Errorf("failed to marshal block: %w", err)
	}

	return c.client.Set(ctx, key, data, c.config.TTL).Err()
}

// GetBlockByHash retrieves a block by hash from cache
func (c *RedisCache) GetBlockByHash(ctx context.Context, hash [32]byte) (*types.Block, error) {
	key := c.key(fmt.Sprintf("block:hash:%x", hash))

	data, err := c.client.Get(ctx, key).Bytes()
	if err == redis.Nil {
		return nil, fmt.Errorf("block not in cache")
	}
	if err != nil {
		return nil, fmt.Errorf("cache error: %w", err)
	}

	var block types.Block
	if err := json.Unmarshal(data, &block); err != nil {
		return nil, fmt.Errorf("failed to unmarshal block: %w", err)
	}

	return &block, nil
}

// SetBlockByHash stores a block by hash in cache
func (c *RedisCache) SetBlockByHash(ctx context.Context, block *types.Block) error {
	key := c.key(fmt.Sprintf("block:hash:%x", block.Header.Hash))

	data, err := json.Marshal(block)
	if err != nil {
		return fmt.Errorf("failed to marshal block: %w", err)
	}

	return c.client.Set(ctx, key, data, c.config.TTL).Err()
}

// GetCheckpoint retrieves a checkpoint from cache
func (c *RedisCache) GetCheckpoint(ctx context.Context, blockNum uint64) (*types.Checkpoint, error) {
	key := c.key(fmt.Sprintf("checkpoint:%d", blockNum))

	data, err := c.client.Get(ctx, key).Bytes()
	if err == redis.Nil {
		return nil, fmt.Errorf("checkpoint not in cache")
	}
	if err != nil {
		return nil, fmt.Errorf("cache error: %w", err)
	}

	var checkpoint types.Checkpoint
	if err := json.Unmarshal(data, &checkpoint); err != nil {
		return nil, fmt.Errorf("failed to unmarshal checkpoint: %w", err)
	}

	return &checkpoint, nil
}

// SetCheckpoint stores a checkpoint in cache
func (c *RedisCache) SetCheckpoint(ctx context.Context, checkpoint *types.Checkpoint) error {
	key := c.key(fmt.Sprintf("checkpoint:%d", checkpoint.BlockNumber))

	data, err := json.Marshal(checkpoint)
	if err != nil {
		return fmt.Errorf("failed to marshal checkpoint: %w", err)
	}

	// Checkpoints have longer TTL (1 day)
	return c.client.Set(ctx, key, data, 24*time.Hour).Err()
}

// GetSyncStatus retrieves sync status from cache (for cluster coordination)
func (c *RedisCache) GetSyncStatus(ctx context.Context, instanceID string) (map[string]interface{}, error) {
	key := c.key(fmt.Sprintf("sync:status:%s", instanceID))

	data, err := c.client.Get(ctx, key).Bytes()
	if err == redis.Nil {
		return nil, fmt.Errorf("status not in cache")
	}
	if err != nil {
		return nil, fmt.Errorf("cache error: %w", err)
	}

	var status map[string]interface{}
	if err := json.Unmarshal(data, &status); err != nil {
		return nil, fmt.Errorf("failed to unmarshal status: %w", err)
	}

	return status, nil
}

// SetSyncStatus stores sync status in cache (for cluster coordination)
func (c *RedisCache) SetSyncStatus(ctx context.Context, instanceID string, status map[string]interface{}) error {
	key := c.key(fmt.Sprintf("sync:status:%s", instanceID))

	data, err := json.Marshal(status)
	if err != nil {
		return fmt.Errorf("failed to marshal status: %w", err)
	}

	// Status expires after 1 minute (heartbeat)
	return c.client.Set(ctx, key, data, 1*time.Minute).Err()
}

// GetAllInstanceStatuses retrieves status for all cluster instances
func (c *RedisCache) GetAllInstanceStatuses(ctx context.Context) (map[string]map[string]interface{}, error) {
	pattern := c.key("sync:status:*")
	keys, err := c.client.Keys(ctx, pattern).Result()
	if err != nil {
		return nil, fmt.Errorf("failed to get keys: %w", err)
	}

	statuses := make(map[string]map[string]interface{})
	for _, key := range keys {
		data, err := c.client.Get(ctx, key).Bytes()
		if err != nil {
			continue
		}

		var status map[string]interface{}
		if err := json.Unmarshal(data, &status); err != nil {
			continue
		}

		// Extract instance ID from key
		instanceID := key[len(c.key("sync:status:")):]
		statuses[instanceID] = status
	}

	return statuses, nil
}

// IncrementCounter increments a counter (for metrics)
func (c *RedisCache) IncrementCounter(ctx context.Context, name string) (int64, error) {
	key := c.key(fmt.Sprintf("counter:%s", name))
	return c.client.Incr(ctx, key).Result()
}

// GetCounter retrieves a counter value
func (c *RedisCache) GetCounter(ctx context.Context, name string) (int64, error) {
	key := c.key(fmt.Sprintf("counter:%s", name))
	val, err := c.client.Get(ctx, key).Int64()
	if err == redis.Nil {
		return 0, nil
	}
	return val, err
}

// SetCounter sets a counter value
func (c *RedisCache) SetCounter(ctx context.Context, name string, value int64) error {
	key := c.key(fmt.Sprintf("counter:%s", name))
	return c.client.Set(ctx, key, value, 0).Err()
}

// PublishEvent publishes an event to a channel (for cluster coordination)
func (c *RedisCache) PublishEvent(ctx context.Context, channel string, message interface{}) error {
	data, err := json.Marshal(message)
	if err != nil {
		return fmt.Errorf("failed to marshal message: %w", err)
	}

	return c.client.Publish(ctx, c.key(channel), data).Err()
}

// SubscribeToEvents subscribes to events on a channel
func (c *RedisCache) SubscribeToEvents(ctx context.Context, channel string) (<-chan []byte, error) {
	pubsub := c.client.Subscribe(ctx, c.key(channel))

	// Wait for confirmation
	if _, err := pubsub.Receive(ctx); err != nil {
		return nil, fmt.Errorf("failed to subscribe: %w", err)
	}

	ch := make(chan []byte, 100)

	go func() {
		defer close(ch)
		defer pubsub.Close()

		msgCh := pubsub.Channel()
		for {
			select {
			case msg := <-msgCh:
				if msg != nil {
					ch <- []byte(msg.Payload)
				}
			case <-ctx.Done():
				return
			}
		}
	}()

	return ch, nil
}

// GetStats returns cache statistics
func (c *RedisCache) GetStats(ctx context.Context) (map[string]interface{}, error) {
	info, err := c.client.Info(ctx, "stats").Result()
	if err != nil {
		return nil, fmt.Errorf("failed to get stats: %w", err)
	}

	poolStats := c.client.PoolStats()

	return map[string]interface{}{
		"redis_info":   info,
		"pool_hits":    poolStats.Hits,
		"pool_misses":  poolStats.Misses,
		"pool_timeout": poolStats.Timeouts,
		"pool_total":   poolStats.TotalConns,
		"pool_idle":    poolStats.IdleConns,
		"pool_stale":   poolStats.StaleConns,
	}, nil
}

// FlushAll clears all cached data (use with caution!)
func (c *RedisCache) FlushAll(ctx context.Context) error {
	// Only flush keys with our prefix
	pattern := c.key("*")
	keys, err := c.client.Keys(ctx, pattern).Result()
	if err != nil {
		return fmt.Errorf("failed to get keys: %w", err)
	}

	if len(keys) > 0 {
		return c.client.Del(ctx, keys...).Err()
	}

	return nil
}

// HealthCheck checks if Redis is healthy
func (c *RedisCache) HealthCheck(ctx context.Context) error {
	return c.client.Ping(ctx).Err()
}

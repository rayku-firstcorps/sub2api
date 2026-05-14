package repository

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/redis/go-redis/v9"
)

const kiroPromptCacheKeyPrefix = "kiro:prompt_cache:"

type kiroPromptCache struct {
	rdb *redis.Client
}

func NewKiroPromptCache(rdb *redis.Client) service.KiroPromptCache {
	if rdb == nil {
		return nil
	}
	return &kiroPromptCache{rdb: rdb}
}

func (c *kiroPromptCache) LookupOrCreate(ctx context.Context, namespace string, breakpoints []service.KiroCacheBreakpoint, totalInputTokens int) (service.KiroCacheResult, error) {
	result := service.KiroCacheResult{UncachedInputTokens: totalInputTokens}
	if c == nil || c.rdb == nil || namespace == "" || len(breakpoints) == 0 {
		return result, nil
	}

	hitIndex := -1
	for i := len(breakpoints) - 1; i >= 0; i-- {
		bp := breakpoints[i]
		key := kiroPromptCacheKey(namespace, bp.Hash)
		val, err := c.rdb.Get(ctx, key).Result()
		if errors.Is(err, redis.Nil) {
			slog.Debug("kiro prompt cache miss", "namespace", namespace, "breakpoint_index", i, "hash", bp.Hash, "tokens", bp.Tokens)
			continue
		}
		if err != nil {
			return result, err
		}
		cachedTokens, err := strconv.Atoi(val)
		if err != nil {
			continue
		}
		result.CacheReadInputTokens = cachedTokens
		hitIndex = i
		slog.Debug("kiro prompt cache hit", "namespace", namespace, "breakpoint_index", i, "hash", bp.Hash, "tokens", cachedTokens)
		if err := c.rdb.Expire(ctx, key, bp.TTL).Err(); err != nil {
			slog.Warn("kiro prompt cache ttl refresh failed", "error", err)
		}
		break
	}

	if hitIndex >= 0 {
		prevTokens := result.CacheReadInputTokens
		for _, bp := range breakpoints[hitIndex+1:] {
			if c.createBreakpoint(ctx, namespace, bp) {
				c.addCreationTokens(&result, bp.Tokens-prevTokens, bp)
			}
			prevTokens = bp.Tokens
		}
	} else {
		prevTokens := 0
		for _, bp := range breakpoints {
			if c.createBreakpoint(ctx, namespace, bp) {
				c.addCreationTokens(&result, bp.Tokens-prevTokens, bp)
			}
			prevTokens = bp.Tokens
		}
	}

	cachedTokens := result.CacheReadInputTokens + result.CacheCreationInputTokens
	result.UncachedInputTokens = totalInputTokens - cachedTokens
	if result.UncachedInputTokens < 0 {
		result.UncachedInputTokens = 0
	}
	return result, nil
}

func (c *kiroPromptCache) createBreakpoint(ctx context.Context, namespace string, bp service.KiroCacheBreakpoint) bool {
	key := kiroPromptCacheKey(namespace, bp.Hash)
	if err := c.rdb.SetEx(ctx, key, bp.Tokens, bp.TTL).Err(); err != nil {
		slog.Warn("kiro prompt cache create failed", "error", err)
		return false
	}
	slog.Debug("kiro prompt cache created", "namespace", namespace, "hash", bp.Hash, "tokens", bp.Tokens, "ttl", bp.TTL)
	return true
}

func (c *kiroPromptCache) addCreationTokens(result *service.KiroCacheResult, tokens int, bp service.KiroCacheBreakpoint) {
	if tokens <= 0 {
		return
	}
	result.CacheCreationInputTokens += tokens
	if bp.TTL >= time.Hour {
		result.CacheCreation1hTokens += tokens
		return
	}
	result.CacheCreation5mTokens += tokens
}

func kiroPromptCacheKey(namespace, hash string) string {
	return fmt.Sprintf("%s%s:%s", kiroPromptCacheKeyPrefix, namespace, hash)
}

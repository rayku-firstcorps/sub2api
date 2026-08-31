package service

import (
	"encoding/json"
	"errors"
	"sort"
	"strings"
	"sync/atomic"
)

const usageLogRequestContextMaxBytes = 256 * 1024
const usageLogRequestContextSkipAPIKeyIDsMax = 1000

// sanitizeAndTrimRequestBody sanitizes and trims a raw request body for usage log storage.
var sanitizeAndTrimRequestBody = sanitizeAndTrimJSONPayload

var usageLogRequestContextEnabled atomic.Bool
var usageLogRequestContextSkipAPIKeyIDs atomic.Value // map[int64]struct{}

var errUsageLogRequestContextSkipAPIKeyIDsTooMany = errors.New(
	"usage log request context skip API key ID list exceeds 1000 entries",
)

func init() {
	usageLogRequestContextEnabled.Store(true)
	usageLogRequestContextSkipAPIKeyIDs.Store(map[int64]struct{}{})
}

// SetUsageLogRequestContextEnabled updates the lock-free hot-path switch after
// settings have been loaded or successfully persisted.
func SetUsageLogRequestContextEnabled(enabled bool) {
	usageLogRequestContextEnabled.Store(enabled)
}

// SetUsageLogRequestContextSkipAPIKeyIDs replaces the lock-free skip set used
// on the usage-log hot path. Duplicate and non-positive IDs are ignored.
func SetUsageLogRequestContextSkipAPIKeyIDs(ids []int64) {
	skip := make(map[int64]struct{}, len(ids))
	for _, id := range ids {
		if id > 0 {
			skip[id] = struct{}{}
		}
	}
	usageLogRequestContextSkipAPIKeyIDs.Store(skip)
}

// ShouldSkipUsageLogRequestContext reports whether this API key is on the
// request-context skip whitelist.
func ShouldSkipUsageLogRequestContext(apiKeyID int64) bool {
	if apiKeyID <= 0 {
		return false
	}
	skip, _ := usageLogRequestContextSkipAPIKeyIDs.Load().(map[int64]struct{})
	if len(skip) == 0 {
		return false
	}
	_, ok := skip[apiKeyID]
	return ok
}

// PrepareUsageLogRequestContext returns a sanitized, bounded JSON snapshot of
// the request body for usage log inspection. Invalid or empty JSON is skipped.
func PrepareUsageLogRequestContext(raw []byte) (jsonString *string, truncated bool, bytesLen *int) {
	if !usageLogRequestContextEnabled.Load() || len(raw) == 0 {
		return nil, false, nil
	}
	out, wasTruncated, originalBytes := sanitizeAndTrimRequestBody(raw, usageLogRequestContextMaxBytes)
	if out == "" {
		if originalBytes > 0 {
			return nil, false, &originalBytes
		}
		return nil, false, nil
	}
	return &out, wasTruncated, &originalBytes
}

// PrepareUsageLogRequestContextForAPIKey skips snapshotting when the API key
// is on the request-context skip whitelist.
func PrepareUsageLogRequestContextForAPIKey(apiKeyID int64, raw []byte) (jsonString *string, truncated bool, bytesLen *int) {
	if ShouldSkipUsageLogRequestContext(apiKeyID) {
		return nil, false, nil
	}
	return PrepareUsageLogRequestContext(raw)
}

// NormalizeUsageLogRequestContextSkipAPIKeyIDs returns a sorted, unique list of
// positive API key IDs. Empty input yields an empty slice, never nil.
func NormalizeUsageLogRequestContextSkipAPIKeyIDs(ids []int64) ([]int64, error) {
	if len(ids) == 0 {
		return []int64{}, nil
	}
	out := make([]int64, 0, len(ids))
	seen := make(map[int64]struct{}, len(ids))
	for _, id := range ids {
		if id <= 0 {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	if len(out) > usageLogRequestContextSkipAPIKeyIDsMax {
		return nil, errUsageLogRequestContextSkipAPIKeyIDsTooMany
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out, nil
}

func parseUsageLogRequestContextSkipAPIKeyIDs(raw string) []int64 {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return []int64{}
	}
	var ids []int64
	if err := json.Unmarshal([]byte(raw), &ids); err != nil {
		return []int64{}
	}
	normalized, err := NormalizeUsageLogRequestContextSkipAPIKeyIDs(ids)
	if err != nil {
		return []int64{}
	}
	return normalized
}

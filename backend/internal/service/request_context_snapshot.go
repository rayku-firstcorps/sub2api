package service

import "sync/atomic"

const usageLogRequestContextMaxBytes = 256 * 1024

// sanitizeAndTrimRequestBody sanitizes and trims a raw request body for usage log storage.
var sanitizeAndTrimRequestBody = sanitizeAndTrimJSONPayload

var usageLogRequestContextEnabled atomic.Bool

func init() {
	usageLogRequestContextEnabled.Store(true)
}

// SetUsageLogRequestContextEnabled updates the lock-free hot-path switch after
// settings have been loaded or successfully persisted.
func SetUsageLogRequestContextEnabled(enabled bool) {
	usageLogRequestContextEnabled.Store(enabled)
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

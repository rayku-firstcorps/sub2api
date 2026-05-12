package service

const usageLogRequestContextMaxBytes = 256 * 1024

// PrepareUsageLogRequestContext returns a sanitized, bounded JSON snapshot of
// the request body for usage log inspection. Invalid or empty JSON is skipped.
func PrepareUsageLogRequestContext(raw []byte) (jsonString *string, truncated bool, bytesLen *int) {
	if len(raw) == 0 {
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

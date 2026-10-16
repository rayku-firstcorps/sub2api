package service

import (
	"context"
	"time"
)

// Public config deliberately contains no reusable credentials.
type OpsFeishuConfig struct {
	Enabled               bool   `json:"enabled"`
	MinSeverity           string `json:"min_severity"`
	RateLimitPerHour      int    `json:"rate_limit_per_hour"`
	IncludeResolvedAlerts bool   `json:"include_resolved_alerts"`
	WebhookConfigured     bool   `json:"webhook_configured"`
	SecretConfigured      bool   `json:"secret_configured"`
}

type OpsFeishuConfigUpdate struct {
	Enabled               bool    `json:"enabled"`
	MinSeverity           string  `json:"min_severity"`
	RateLimitPerHour      int     `json:"rate_limit_per_hour"`
	IncludeResolvedAlerts bool    `json:"include_resolved_alerts"`
	WebhookURL            *string `json:"webhook_url"`
	SigningSecret         *string `json:"signing_secret"`
	ClearCredentials      bool    `json:"clear_credentials"`
}

type opsFeishuStoredConfig struct {
	OpsFeishuConfig
	WebhookCiphertext string `json:"webhook_ciphertext"`
	SecretCiphertext  string `json:"secret_ciphertext"`
	Version           string `json:"version"`
}

type OpsFeishuDelivery struct {
	ID            int64      `json:"id"`
	EventID       int64      `json:"event_id"`
	Kind          string     `json:"kind"`
	Status        string     `json:"status"`
	Attempts      int        `json:"attempts"`
	LastError     string     `json:"last_error,omitempty"`
	SentAt        *time.Time `json:"sent_at,omitempty"`
	ConfigVersion string     `json:"-"`
	LeaseToken    string     `json:"-"`
}

// Kept separate from OpsRepository so non-notification consumers need not implement it.
type OpsFeishuRepository interface {
	CreateAlertEventWithFeishu(context.Context, *OpsAlertEvent, string) (*OpsAlertEvent, error)
	ResolveAlertEventWithFeishu(context.Context, int64, time.Time, string) error
	ClaimFeishuDelivery(context.Context) (*OpsFeishuDelivery, error)
	FinishFeishuDelivery(context.Context, *OpsFeishuDelivery, string, string, time.Time, bool) error
	ListFeishuDeliveries(context.Context, []int64) ([]*OpsFeishuDelivery, error)
	ReserveFeishuSend(context.Context, int, bool) (bool, error)
}

package service

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"sync"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/google/uuid"
)

const SettingKeyOpsFeishuNotificationConfig = "ops_feishu_notification_config"

type OpsFeishuService struct {
	ops       *OpsService
	repo      OpsFeishuRepository
	encryptor SecretEncryptor
	client    *http.Client
	configMu  sync.Mutex
}

func NewOpsFeishuService(ops *OpsService, encryptor SecretEncryptor) *OpsFeishuService {
	repo, _ := ops.opsRepo.(OpsFeishuRepository)
	return &OpsFeishuService{ops: ops, repo: repo, encryptor: encryptor, client: newFeishuHTTPClient()}
}

func (s *OpsFeishuService) loadConfig(ctx context.Context) (*opsFeishuStoredConfig, error) {
	cfg := &opsFeishuStoredConfig{OpsFeishuConfig: OpsFeishuConfig{RateLimitPerHour: 60}}
	if s == nil || s.ops == nil || s.ops.settingRepo == nil {
		return nil, errors.New("Feishu settings unavailable")
	}
	raw, err := s.ops.settingRepo.GetValue(ctx, SettingKeyOpsFeishuNotificationConfig)
	if errors.Is(err, ErrSettingNotFound) {
		return cfg, nil
	}
	if err != nil {
		return nil, errors.New("Failed to load Feishu settings")
	}
	if json.Unmarshal([]byte(raw), cfg) != nil {
		return nil, errors.New("Invalid stored Feishu settings")
	}
	cfg.WebhookConfigured = cfg.WebhookCiphertext != ""
	cfg.SecretConfigured = cfg.SecretCiphertext != ""
	return cfg, nil
}

func (s *OpsService) GetFeishuNotificationConfig(ctx context.Context) (*OpsFeishuConfig, error) {
	if s.feishu == nil {
		return nil, errors.New("Feishu service unavailable")
	}
	cfg, err := s.feishu.loadConfig(ctx)
	if err != nil {
		return nil, err
	}
	return &cfg.OpsFeishuConfig, nil
}

func (s *OpsService) UpdateFeishuNotificationConfig(ctx context.Context, req *OpsFeishuConfigUpdate) (*OpsFeishuConfig, error) {
	if s.feishu == nil {
		return nil, errors.New("Feishu service unavailable")
	}
	return s.feishu.updateConfig(ctx, req)
}

func feishuConfigError(message string) error {
	return infraerrors.BadRequest("INVALID_FEISHU_CONFIG", message)
}

func (s *OpsFeishuService) updateConfig(ctx context.Context, req *OpsFeishuConfigUpdate) (*OpsFeishuConfig, error) {
	if req == nil {
		return nil, feishuConfigError("Invalid request")
	}
	if req.RateLimitPerHour < 1 || req.RateLimitPerHour > 3600 {
		return nil, feishuConfigError("rate_limit_per_hour must be between 1 and 3600")
	}
	severity := strings.TrimSpace(req.MinSeverity)
	switch severity {
	case "", "info", "warning", "critical":
	default:
		return nil, feishuConfigError("Invalid min_severity")
	}
	s.configMu.Lock()
	defer s.configMu.Unlock()
	cfg, err := s.loadConfig(ctx)
	if err != nil {
		return nil, err
	}
	if req.ClearCredentials {
		if req.Enabled || req.WebhookURL != nil || req.SigningSecret != nil {
			return nil, feishuConfigError("Disable Feishu and omit credentials before clearing")
		}
		cfg.WebhookCiphertext, cfg.SecretCiphertext = "", ""
		cfg.Version = uuid.NewString()
	} else {
		if req.WebhookURL != nil {
			webhook := strings.TrimSpace(*req.WebhookURL)
			if validateFeishuWebhook(webhook) != nil {
				return nil, feishuConfigError("Invalid Feishu webhook URL")
			}
			// A new destination must explicitly supply its own signing secret.
			if req.SigningSecret == nil {
				return nil, feishuConfigError("A signing secret is required when replacing the webhook")
			}
			if s.encryptor == nil {
				return nil, errors.New("Credential encryption unavailable")
			}
			cfg.WebhookCiphertext, err = s.encryptor.Encrypt(webhook)
			if err != nil {
				return nil, errors.New("Failed to encrypt Feishu credentials")
			}
			cfg.Version = uuid.NewString()
		}
		if req.SigningSecret != nil {
			secret := strings.TrimSpace(*req.SigningSecret)
			if secret == "" || len(secret) > 512 {
				return nil, feishuConfigError("A non-empty signing secret (up to 512 bytes) is required")
			}
			if s.encryptor == nil {
				return nil, errors.New("Credential encryption unavailable")
			}
			cfg.SecretCiphertext, err = s.encryptor.Encrypt(secret)
			if err != nil {
				return nil, errors.New("Failed to encrypt Feishu credentials")
			}
		}
	}
	if (cfg.WebhookCiphertext == "") != (cfg.SecretCiphertext == "") || (req.Enabled && cfg.WebhookCiphertext == "") {
		return nil, feishuConfigError("Webhook and signing secret are both required")
	}
	cfg.OpsFeishuConfig = OpsFeishuConfig{Enabled: req.Enabled, MinSeverity: severity,
		RateLimitPerHour: req.RateLimitPerHour, IncludeResolvedAlerts: req.IncludeResolvedAlerts,
		WebhookConfigured: cfg.WebhookCiphertext != "", SecretConfigured: cfg.SecretCiphertext != ""}
	raw, err := json.Marshal(cfg)
	if err != nil {
		return nil, err
	}
	if err := s.ops.settingRepo.Set(ctx, SettingKeyOpsFeishuNotificationConfig, string(raw)); err != nil {
		return nil, errors.New("Failed to save Feishu settings")
	}
	return &cfg.OpsFeishuConfig, nil
}

func (s *OpsFeishuService) credentials(cfg *opsFeishuStoredConfig) (string, string, error) {
	if s.encryptor == nil || cfg.WebhookCiphertext == "" || cfg.SecretCiphertext == "" {
		return "", "", errors.New("Webhook and signing secret are required")
	}
	webhook, err := s.encryptor.Decrypt(cfg.WebhookCiphertext)
	if err != nil {
		return "", "", errors.New("Failed to decrypt Feishu credentials")
	}
	secret, err := s.encryptor.Decrypt(cfg.SecretCiphertext)
	if err != nil {
		return "", "", errors.New("Failed to decrypt Feishu credentials")
	}
	if validateFeishuWebhook(webhook) != nil || strings.TrimSpace(secret) == "" {
		return "", "", errors.New("Invalid Feishu credentials; signing is mandatory")
	}
	return webhook, secret, nil
}

package service

import (
	"context"
	"errors"
	"log/slog"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	kiroTokenRefreshSkew      = 3 * time.Minute
	kiroTokenCacheSkew        = 5 * time.Minute
	kiroRequestRefreshTimeout = 8 * time.Second
)

type KiroTokenProvider struct {
	accountRepo      AccountRepository
	tokenCache       GeminiTokenCache
	refreshAPI       *OAuthRefreshAPI
	executor         OAuthRefreshExecutor
	refreshPolicy    ProviderRefreshPolicy
	tempUnschedCache TempUnschedCache
	mu               sync.Mutex
}

func NewKiroTokenProvider(
	accountRepo AccountRepository,
	tokenCache GeminiTokenCache,
) *KiroTokenProvider {
	return &KiroTokenProvider{
		accountRepo:   accountRepo,
		tokenCache:    tokenCache,
		refreshPolicy: KiroProviderRefreshPolicy(),
	}
}

func (p *KiroTokenProvider) SetRefreshAPI(api *OAuthRefreshAPI, executor OAuthRefreshExecutor) {
	p.refreshAPI = api
	p.executor = executor
}

func (p *KiroTokenProvider) SetRefreshPolicy(policy ProviderRefreshPolicy) {
	p.refreshPolicy = policy
}

func (p *KiroTokenProvider) SetTempUnschedCache(cache TempUnschedCache) {
	p.tempUnschedCache = cache
}

func (p *KiroTokenProvider) GetAccessToken(ctx context.Context, account *Account) (string, error) {
	if account == nil {
		return "", errors.New("account is nil")
	}
	if account.Platform != PlatformKiro {
		return "", errors.New("not a kiro account")
	}
	if account.Type != AccountTypeOAuth {
		return "", errors.New("not a kiro oauth account")
	}

	proxyFingerprint, err := kiroProxyFingerprint(account)
	if err != nil {
		return "", err
	}
	cacheKey := kiroTokenCacheKey(account, proxyFingerprint)

	if p.tokenCache != nil {
		if token, err := p.tokenCache.GetAccessToken(ctx, cacheKey); err == nil && strings.TrimSpace(token) != "" {
			return token, nil
		}
	}

	expiresAt := account.GetCredentialAsTime("expires_at")
	proxyMismatch := !kiroTokenProxyFingerprintMatches(account, proxyFingerprint)
	needsRefresh := expiresAt == nil ||
		time.Until(*expiresAt) <= kiroTokenRefreshSkew ||
		proxyMismatch
	if needsRefresh && p.refreshAPI != nil && p.executor != nil {
		refreshCtx, cancel := context.WithTimeout(ctx, kiroRequestRefreshTimeout)
		defer cancel()
		result, err := p.refreshAPI.RefreshIfNeeded(refreshCtx, account, p.executor, kiroTokenRefreshSkew)
		if err != nil {
			p.markTempUnschedulable(account, err)
			if proxyMismatch {
				return "", err
			}
			if p.refreshPolicy.OnRefreshError == ProviderRefreshErrorReturn {
				return "", err
			}
		} else if result.LockHeld {
			if p.tokenCache != nil {
				time.Sleep(200 * time.Millisecond)
				if token, cacheErr := p.tokenCache.GetAccessToken(ctx, cacheKey); cacheErr == nil && strings.TrimSpace(token) != "" {
					return token, nil
				}
			}
			if proxyMismatch {
				return "", errors.New("kiro access_token proxy mismatch and refresh lock is held")
			}
		} else {
			account = result.Account
			expiresAt = account.GetCredentialAsTime("expires_at")
			if refreshedFingerprint, fpErr := kiroProxyFingerprint(account); fpErr == nil && refreshedFingerprint != "" {
				proxyFingerprint = refreshedFingerprint
				cacheKey = kiroTokenCacheKey(account, proxyFingerprint)
			}
			proxyMismatch = !kiroTokenProxyFingerprintMatches(account, proxyFingerprint)
		}
	}

	if proxyMismatch {
		return "", errors.New("kiro access_token was not refreshed through the current account proxy")
	}

	accessToken := account.GetCredential("access_token")
	if strings.TrimSpace(accessToken) == "" {
		return "", errors.New("access_token not found in credentials")
	}

	if p.tokenCache != nil {
		ttl := 30 * time.Minute
		if expiresAt != nil {
			until := time.Until(*expiresAt)
			switch {
			case until > kiroTokenCacheSkew:
				ttl = until - kiroTokenCacheSkew
			case until > 0:
				ttl = until
			default:
				ttl = time.Minute
			}
		}
		_ = p.tokenCache.SetAccessToken(ctx, cacheKey, accessToken, ttl)
	}

	return accessToken, nil
}

func (p *KiroTokenProvider) markTempUnschedulable(account *Account, refreshErr error) {
	if p.accountRepo == nil || account == nil {
		return
	}
	now := time.Now()
	until := now.Add(tokenRefreshTempUnschedDuration)
	reason := "kiro token refresh failed: " + refreshErr.Error()

	bgCtx := context.Background()
	if err := p.accountRepo.SetTempUnschedulable(bgCtx, account.ID, until, reason); err != nil {
		slog.Warn("kiro_token_provider.set_temp_unschedulable_failed",
			"account_id", account.ID,
			"error", err,
		)
		return
	}
	slog.Warn("kiro_token_provider.temp_unschedulable_set",
		"account_id", account.ID,
		"until", until.Format(time.RFC3339),
		"reason", reason,
	)
	if p.tempUnschedCache != nil {
		state := &TempUnschedState{
			UntilUnix:       until.Unix(),
			TriggeredAtUnix: now.Unix(),
			ErrorMessage:    reason,
		}
		if err := p.tempUnschedCache.SetTempUnsched(bgCtx, account.ID, state); err != nil {
			slog.Warn("kiro_token_provider.temp_unsched_cache_set_failed",
				"account_id", account.ID,
				"error", err,
			)
		}
	}
}

func KiroTokenCacheKey(account *Account) string {
	return "kiro:account:" + strconv.FormatInt(account.ID, 10)
}

func kiroTokenCacheKey(account *Account, proxyFingerprint string) string {
	base := "kiro:account:" + strings.TrimSpace(account.GetCredential("profile_arn"))
	if base == "kiro:account:" {
		base = KiroTokenCacheKey(account)
	}
	proxyFingerprint = strings.TrimSpace(proxyFingerprint)
	if proxyFingerprint == "" {
		proxyFingerprint = kiroProxyFingerprintNone
	}
	return base + ":proxy:" + proxyFingerprint
}

func kiroTokenProxyFingerprint(account *Account) string {
	if account == nil {
		return ""
	}
	return strings.TrimSpace(account.GetCredential(kiroProxyFingerprintCredentialKey))
}

func kiroTokenProxyFingerprintMatches(account *Account, current string) bool {
	stored := kiroTokenProxyFingerprint(account)
	current = strings.TrimSpace(current)
	if current == "" {
		current = kiroProxyFingerprintNone
	}
	if stored == "" {
		return current == kiroProxyFingerprintNone
	}
	return stored == current
}

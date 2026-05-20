package service

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type kiroTokenCacheStub struct {
	tokens map[string]string
	gets   []string
	sets   []string
}

func (s *kiroTokenCacheStub) GetAccessToken(_ context.Context, cacheKey string) (string, error) {
	s.gets = append(s.gets, cacheKey)
	if s.tokens == nil {
		return "", errors.New("not cached")
	}
	token := s.tokens[cacheKey]
	if token == "" {
		return "", errors.New("not cached")
	}
	return token, nil
}

func (s *kiroTokenCacheStub) SetAccessToken(_ context.Context, cacheKey string, _ string, _ time.Duration) error {
	s.sets = append(s.sets, cacheKey)
	return nil
}

func (s *kiroTokenCacheStub) DeleteAccessToken(_ context.Context, _ string) error {
	return nil
}

func (s *kiroTokenCacheStub) AcquireRefreshLock(_ context.Context, _ string, _ time.Duration) (bool, error) {
	return true, nil
}

func (s *kiroTokenCacheStub) ReleaseRefreshLock(_ context.Context, _ string) error {
	return nil
}

func TestKiroAccountProxyURLPrefersBoundProxy(t *testing.T) {
	t.Parallel()

	proxyID := int64(7)
	account := &Account{
		ID:      42,
		ProxyID: &proxyID,
		Proxy: &Proxy{
			ID:       proxyID,
			Protocol: "socks5",
			Host:     "proxy.example.com",
			Port:     1080,
			Username: "user",
			Password: "pass",
		},
		Credentials: map[string]any{
			"proxy_url": "http://legacy.example.com:8080",
		},
	}

	got, err := kiroAccountProxyURL(account)

	require.NoError(t, err)
	require.Equal(t, "socks5://user:pass@proxy.example.com:1080", got)
}

func TestKiroAccountProxyURLErrorsWhenBoundProxyNotLoaded(t *testing.T) {
	t.Parallel()

	proxyID := int64(7)
	account := &Account{
		ID:      42,
		ProxyID: &proxyID,
		Credentials: map[string]any{
			"proxy_url": "http://legacy.example.com:8080",
		},
	}

	got, err := kiroAccountProxyURL(account)

	require.Error(t, err)
	require.Empty(t, got)
	require.Contains(t, err.Error(), "proxy is not loaded")
}

func TestKiroAccountProxyURLErrorsWithoutBoundProxy(t *testing.T) {
	t.Parallel()

	account := &Account{
		ID: 42,
		Credentials: map[string]any{
			"proxy_url": " http://legacy.example.com:8080 ",
		},
	}

	got, err := kiroAccountProxyURL(account)

	require.Error(t, err)
	require.Empty(t, got)
	require.Contains(t, err.Error(), "has no bound proxy")
}

func TestKiroAccountProxyURLForOperationLogsSanitizedProxy(t *testing.T) {
	var buf strings.Builder
	logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo}))
	previous := slog.Default()
	slog.SetDefault(logger)
	t.Cleanup(func() { slog.SetDefault(previous) })

	proxyID := int64(7)
	account := &Account{
		ID:      42,
		ProxyID: &proxyID,
		Proxy: &Proxy{
			ID:       proxyID,
			Protocol: "http",
			Host:     "proxy.example.com",
			Port:     8080,
			Username: "user",
			Password: "secret",
		},
	}

	got, err := kiroAccountProxyURLForOperation(account, "gateway_generate")

	require.NoError(t, err)
	require.Equal(t, "http://user:secret@proxy.example.com:8080", got)
	logOutput := buf.String()
	require.Contains(t, logOutput, "kiro.proxy_resolved")
	require.Contains(t, logOutput, "operation=gateway_generate")
	require.Contains(t, logOutput, "proxy_enabled=true")
	require.Contains(t, logOutput, "proxy_source=account_proxy")
	require.Contains(t, logOutput, "proxy_id=7")
	require.Contains(t, logOutput, "proxy_host=proxy.example.com")
	require.NotContains(t, logOutput, "secret")
	require.NotContains(t, logOutput, "user:secret")
}

func TestKiroAccountProxyURLForOperationLogsResolveFailure(t *testing.T) {
	var buf strings.Builder
	logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo}))
	previous := slog.Default()
	slog.SetDefault(logger)
	t.Cleanup(func() { slog.SetDefault(previous) })

	account := &Account{ID: 42}

	got, err := kiroAccountProxyURLForOperation(account, "account_test_generate")

	require.Error(t, err)
	require.Empty(t, got)
	logOutput := buf.String()
	require.Contains(t, logOutput, "kiro.proxy_resolve_failed")
	require.Contains(t, logOutput, "operation=account_test_generate")
	require.Contains(t, logOutput, "proxy_enabled=false")
	require.Contains(t, logOutput, "proxy_source=none")
	require.Contains(t, logOutput, "account_id=42")
	require.Contains(t, logOutput, "has no bound proxy")
}

func TestKiroTokenCacheKeyIncludesProxyFingerprint(t *testing.T) {
	t.Parallel()

	proxyID := int64(7)
	account := &Account{
		ID:      42,
		ProxyID: &proxyID,
		Proxy: &Proxy{
			ID:       proxyID,
			Protocol: "http",
			Host:     "proxy.example.com",
			Port:     8080,
		},
		Credentials: map[string]any{
			"profile_arn": "profile-a",
		},
	}
	fingerprint, err := kiroProxyFingerprint(account)
	require.NoError(t, err)

	require.Equal(t, "kiro:account:profile-a:proxy:"+fingerprint, kiroTokenCacheKey(account, fingerprint))
}

func TestKiroTokenProviderDoesNotReuseTokenFromDifferentProxy(t *testing.T) {
	t.Parallel()

	proxyID := int64(7)
	account := &Account{
		ID:       42,
		Platform: PlatformKiro,
		Type:     AccountTypeOAuth,
		ProxyID:  &proxyID,
		Proxy: &Proxy{
			ID:       proxyID,
			Protocol: "http",
			Host:     "proxy.example.com",
			Port:     8080,
		},
		Credentials: map[string]any{
			"access_token": "old-token",
			"expires_at":   time.Now().Add(time.Hour).Format(time.RFC3339),
		},
	}
	cache := &kiroTokenCacheStub{tokens: map[string]string{
		KiroTokenCacheKey(account): "cached-old-token",
	}}
	fingerprint, err := kiroProxyFingerprint(account)
	require.NoError(t, err)
	provider := NewKiroTokenProvider(nil, cache)

	got, err := provider.GetAccessToken(context.Background(), account)

	require.Error(t, err)
	require.Empty(t, got)
	require.Contains(t, err.Error(), "current account proxy")
	require.Equal(t, []string{"kiro:account:42:proxy:" + fingerprint}, cache.gets)
	require.Empty(t, cache.sets)
}

func TestKiroTokenRefresherNeedsRefreshWhenProxyFingerprintChanged(t *testing.T) {
	t.Parallel()

	proxyID := int64(7)
	account := &Account{
		ID:       42,
		Platform: PlatformKiro,
		Type:     AccountTypeOAuth,
		ProxyID:  &proxyID,
		Proxy: &Proxy{
			ID:       proxyID,
			Protocol: "http",
			Host:     "proxy.example.com",
			Port:     8080,
		},
		Credentials: map[string]any{
			"access_token":                    "old-token",
			"refresh_token":                   "refresh-token",
			"expires_at":                      time.Now().Add(time.Hour).Format(time.RFC3339),
			kiroProxyFingerprintCredentialKey: "account_proxy:6:oldproxy",
		},
	}

	require.True(t, NewKiroTokenRefresher().NeedsRefresh(account, time.Hour))
}

type kiroGatewayProxyRecordingUpstream struct {
	proxyURL string
}

func (u *kiroGatewayProxyRecordingUpstream) Do(req *http.Request, proxyURL string, accountID int64, accountConcurrency int) (*http.Response, error) {
	u.proxyURL = proxyURL
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(`{"content":"ok"}{"stop":true}`)),
	}, nil
}

func (u *kiroGatewayProxyRecordingUpstream) DoWithTLS(req *http.Request, proxyURL string, accountID int64, accountConcurrency int, profile *tlsfingerprint.Profile) (*http.Response, error) {
	return u.Do(req, proxyURL, accountID, accountConcurrency)
}

func TestKiroGatewayForwardUsesBoundProxy(t *testing.T) {
	gin.SetMode(gin.TestMode)

	proxyID := int64(7)
	expiresAt := time.Now().Add(time.Hour).Format(time.RFC3339)
	account := &Account{
		ID:       42,
		Platform: PlatformKiro,
		Type:     AccountTypeOAuth,
		ProxyID:  &proxyID,
		Proxy: &Proxy{
			ID:       proxyID,
			Protocol: "http",
			Host:     "proxy.example.com",
			Port:     8080,
		},
		Credentials: map[string]any{
			"access_token": "token",
			"expires_at":   expiresAt,
			"proxy_url":    "http://legacy.example.com:8080",
		},
	}
	fingerprint, err := kiroProxyFingerprint(account)
	require.NoError(t, err)
	account.Credentials[kiroProxyFingerprintCredentialKey] = fingerprint
	upstream := &kiroGatewayProxyRecordingUpstream{}
	svc := &KiroGatewayService{
		tokenProvider: NewKiroTokenProvider(nil, nil),
		httpUpstream:  upstream,
	}
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
	body := []byte(`{"model":"claude-sonnet-4-5","messages":[{"role":"user","content":"hello"}]}`)

	result, err := svc.Forward(context.Background(), c, account, body, false, nil)

	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, "http://proxy.example.com:8080", upstream.proxyURL)
}

func TestKiroTokenRefresherUsesBoundProxy(t *testing.T) {
	t.Parallel()

	proxyHits := make(chan string, 1)
	proxyServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		proxyHits <- r.Method + " " + r.Host
		w.WriteHeader(http.StatusOK)
	}))
	defer proxyServer.Close()

	proxyAddr := strings.TrimPrefix(proxyServer.URL, "http://")
	host, portText, err := net.SplitHostPort(proxyAddr)
	require.NoError(t, err)
	port, err := strconv.Atoi(portText)
	require.NoError(t, err)

	proxyID := int64(7)
	account := &Account{
		ID:       42,
		Platform: PlatformKiro,
		Type:     AccountTypeOAuth,
		ProxyID:  &proxyID,
		Proxy: &Proxy{
			ID:       proxyID,
			Protocol: "http",
			Host:     host,
			Port:     port,
		},
		Credentials: map[string]any{
			"refresh_token": "old-refresh",
			"auth_method":   "social",
			"region":        "us-east-1",
			"proxy_url":     "http://legacy.example.com:8080",
		},
	}

	_, err = NewKiroTokenRefresher().Refresh(context.Background(), account)

	require.Error(t, err)
	select {
	case hit := <-proxyHits:
		require.Equal(t, "CONNECT prod.us-east-1.auth.desktop.kiro.dev:443", hit)
	case <-time.After(time.Second):
		t.Fatal("expected refresh request through bound proxy")
	}
}

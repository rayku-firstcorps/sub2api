package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/kiro"
	"github.com/Wei-Shaw/sub2api/internal/pkg/proxyurl"
	"github.com/Wei-Shaw/sub2api/internal/pkg/proxyutil"
)

const (
	kiroRefreshWindow  = 10 * time.Minute
	kiroRefreshTimeout = 10 * time.Second
)

type KiroTokenRefresher struct{}

func NewKiroTokenRefresher() *KiroTokenRefresher {
	return &KiroTokenRefresher{}
}

func (r *KiroTokenRefresher) CacheKey(account *Account) string {
	return "kiro:account:" + strconv.FormatInt(account.ID, 10)
}

func (r *KiroTokenRefresher) CanRefresh(account *Account) bool {
	return account.Platform == PlatformKiro && account.Type == AccountTypeOAuth
}

func (r *KiroTokenRefresher) NeedsRefresh(account *Account, _ time.Duration) bool {
	if !r.CanRefresh(account) {
		return false
	}
	currentFingerprint, err := kiroProxyFingerprint(account)
	if err != nil {
		return true
	}
	if !kiroTokenProxyFingerprintMatches(account, currentFingerprint) {
		return true
	}
	expiresAt := account.GetCredentialAsTime("expires_at")
	if expiresAt == nil {
		return true
	}
	return time.Until(*expiresAt) < kiroRefreshWindow
}

func (r *KiroTokenRefresher) Refresh(ctx context.Context, account *Account) (map[string]any, error) {
	refreshToken := account.GetCredential("refresh_token")
	if strings.TrimSpace(refreshToken) == "" {
		return nil, fmt.Errorf("kiro account missing refresh_token")
	}

	authMethod := account.GetCredential("auth_method")
	if authMethod == "" {
		authMethod = kiro.AuthMethodSocial
	}
	region := account.GetCredential("region")
	if region == "" {
		region = kiro.DefaultRegion
	}

	proxyResolution, err := kiroAccountProxyResolutionForOperation(account, "token_refresh")
	if err != nil {
		return nil, fmt.Errorf("resolve kiro proxy: %w", err)
	}

	client, err := newKiroHTTPClient(proxyResolution.URL)
	if err != nil {
		return nil, fmt.Errorf("create http client: %w", err)
	}

	var newCreds map[string]any

	switch authMethod {
	case kiro.AuthMethodBuilderID:
		newCreds, err = refreshKiroBuilderID(ctx, client, account, region)
	default:
		newCreds, err = refreshKiroSocial(ctx, client, refreshToken, region)
	}
	if err != nil {
		return nil, err
	}

	newCreds["auth_method"] = authMethod
	newCreds["region"] = region
	newCreds[kiroProxyFingerprintCredentialKey] = proxyResolution.Fingerprint
	if profileArn := account.GetCredential("profile_arn"); profileArn != "" {
		if _, exists := newCreds["profile_arn"]; !exists {
			newCreds["profile_arn"] = profileArn
		}
	}

	return MergeCredentials(account.Credentials, newCreds), nil
}

func refreshKiroSocial(ctx context.Context, client *http.Client, refreshToken, region string) (map[string]any, error) {
	endpoint := kiro.GetRefreshURL(kiro.AuthMethodSocial, region)
	body, _ := json.Marshal(kiro.SocialRefreshRequest{RefreshToken: refreshToken})

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("kiro social refresh request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("kiro social refresh failed (HTTP %d): %s", resp.StatusCode, string(respBody))
	}

	var result kiro.SocialRefreshResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("kiro social refresh parse error: %w", err)
	}

	now := time.Now()
	expiresAt := now.Add(time.Duration(result.ExpiresIn) * time.Second)

	creds := map[string]any{
		"access_token":  result.AccessToken,
		"refresh_token": result.RefreshToken,
		"expires_at":    expiresAt.Format(time.RFC3339),
	}
	if result.ProfileArn != "" {
		creds["profile_arn"] = result.ProfileArn
	}
	return creds, nil
}

func refreshKiroBuilderID(ctx context.Context, client *http.Client, account *Account, region string) (map[string]any, error) {
	refreshToken := account.GetCredential("refresh_token")
	clientID := account.GetCredential("client_id")
	clientSecret := account.GetCredential("client_secret")

	if refreshToken == "" || clientID == "" || clientSecret == "" {
		return nil, fmt.Errorf("kiro builder_id account missing required credentials")
	}

	endpoint := kiro.GetRefreshURL(kiro.AuthMethodBuilderID, region)
	body, _ := json.Marshal(kiro.BuilderIDRefreshRequest{
		ClientID:     clientID,
		ClientSecret: clientSecret,
		RefreshToken: refreshToken,
		GrantType:    "refresh_token",
	})

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("kiro builder_id refresh request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("kiro builder_id refresh failed (HTTP %d): %s", resp.StatusCode, string(respBody))
	}

	var result kiro.BuilderIDRefreshResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("kiro builder_id refresh parse error: %w", err)
	}

	now := time.Now()
	expiresAt := now.Add(time.Duration(result.ExpiresIn) * time.Second)

	creds := map[string]any{
		"access_token":  result.AccessToken,
		"expires_at":    expiresAt.Format(time.RFC3339),
		"client_id":     clientID,
		"client_secret": clientSecret,
	}
	if result.RefreshToken != "" {
		creds["refresh_token"] = result.RefreshToken
	} else {
		creds["refresh_token"] = refreshToken
	}
	return creds, nil
}

func newKiroHTTPClient(proxyURL string) (*http.Client, error) {
	client := &http.Client{
		Timeout: kiroRefreshTimeout,
	}

	if proxyURL == "" {
		return client, nil
	}

	_, parsed, err := proxyurl.Parse(proxyURL)
	if err != nil {
		return nil, err
	}
	if parsed != nil {
		transport := &http.Transport{
			DialContext: (&net.Dialer{
				Timeout: 5 * time.Second,
			}).DialContext,
			TLSHandshakeTimeout: 5 * time.Second,
		}
		if err := proxyutil.ConfigureTransportProxy(transport, parsed); err != nil {
			return nil, fmt.Errorf("configure proxy: %w", err)
		}
		client.Transport = transport
	}

	return client, nil
}

package service

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
)

type kiroProxyResolution struct {
	URL         string
	Enabled     bool
	Source      string
	ProxyID     int64
	HasProxyID  bool
	Protocol    string
	Host        string
	Port        int
	Fingerprint string
}

func kiroAccountProxyURL(account *Account) (string, error) {
	resolution, err := resolveKiroAccountProxy(account)
	if err != nil {
		return "", err
	}
	return resolution.URL, nil
}

func kiroAccountProxyURLForOperation(account *Account, operation string) (string, error) {
	resolution, err := kiroAccountProxyResolutionForOperation(account, operation)
	if err != nil {
		return "", err
	}
	return resolution.URL, nil
}

func kiroAccountProxyResolutionForOperation(account *Account, operation string) (kiroProxyResolution, error) {
	resolution, err := resolveKiroAccountProxy(account)
	if err != nil {
		logKiroProxyResolveFailed(account, operation, err)
		return kiroProxyResolution{}, err
	}
	logKiroProxyResolution(account, operation, resolution)
	return resolution, nil
}

func resolveKiroAccountProxy(account *Account) (kiroProxyResolution, error) {
	if account == nil {
		return kiroProxyResolution{}, fmt.Errorf("kiro account is nil")
	}
	if account.ProxyID == nil {
		return kiroProxyResolution{}, fmt.Errorf("kiro account %d has no bound proxy", account.ID)
	}
	if account.Proxy == nil {
		return kiroProxyResolution{}, fmt.Errorf("kiro account %d has proxy_id %d but proxy is not loaded", account.ID, *account.ProxyID)
	}
	if strings.TrimSpace(account.Proxy.Protocol) == "" || strings.TrimSpace(account.Proxy.Host) == "" || account.Proxy.Port <= 0 {
		return kiroProxyResolution{}, fmt.Errorf("kiro account %d has incomplete bound proxy %d", account.ID, *account.ProxyID)
	}
	proxyURL := strings.TrimSpace(account.Proxy.URL())
	if proxyURL == "" {
		return kiroProxyResolution{}, fmt.Errorf("kiro account %d bound proxy %d resolved empty url", account.ID, *account.ProxyID)
	}
	return kiroProxyResolution{
		URL:         proxyURL,
		Enabled:     true,
		Source:      "account_proxy",
		ProxyID:     *account.ProxyID,
		HasProxyID:  true,
		Protocol:    account.Proxy.Protocol,
		Host:        account.Proxy.Host,
		Port:        account.Proxy.Port,
		Fingerprint: kiroProxyFingerprintForBoundProxy(*account.ProxyID, proxyURL),
	}, nil
}

const (
	kiroProxyFingerprintNone          = "none"
	kiroProxyFingerprintCredentialKey = "_kiro_proxy_fingerprint"
)

func kiroProxyFingerprint(account *Account) (string, error) {
	resolution, err := resolveKiroAccountProxy(account)
	if err != nil {
		return "", err
	}
	if resolution.Fingerprint == "" {
		return kiroProxyFingerprintNone, nil
	}
	return resolution.Fingerprint, nil
}

func kiroProxyFingerprintForBoundProxy(proxyID int64, proxyURL string) string {
	return "account_proxy:" + strconv.FormatInt(proxyID, 10) + ":" + kiroProxyURLHash(proxyURL)
}

func kiroProxyURLHash(proxyURL string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(proxyURL)))
	return hex.EncodeToString(sum[:])[:16]
}

func normalizeKiroProxyOperation(operation string) string {
	operation = strings.TrimSpace(operation)
	if operation == "" {
		return "unknown"
	}
	return operation
}

func logKiroProxyResolveFailed(account *Account, operation string, err error) {
	operation = normalizeKiroProxyOperation(operation)
	proxySource := "none"
	if account != nil && account.ProxyID != nil {
		proxySource = "account_proxy"
	}
	attrs := []any{
		"component", "service.kiro_proxy",
		"operation", operation,
		"proxy_enabled", false,
		"proxy_source", proxySource,
	}
	if account != nil {
		attrs = append(attrs, "account_id", account.ID)
		if account.ProxyID != nil {
			attrs = append(attrs, "proxy_id", *account.ProxyID)
		}
	}
	if err != nil {
		attrs = append(attrs, "reason", err.Error())
	}
	slog.Warn("kiro.proxy_resolve_failed", attrs...)
}

func logKiroProxyResolution(account *Account, operation string, resolution kiroProxyResolution) {
	operation = normalizeKiroProxyOperation(operation)
	attrs := []any{
		"component", "service.kiro_proxy",
		"operation", operation,
		"proxy_enabled", resolution.Enabled,
		"proxy_source", resolution.Source,
	}
	if account != nil {
		attrs = append(attrs, "account_id", account.ID)
	}
	if resolution.HasProxyID {
		attrs = append(attrs, "proxy_id", resolution.ProxyID)
	}
	if resolution.Protocol != "" {
		attrs = append(attrs, "proxy_protocol", resolution.Protocol)
	}
	if resolution.Host != "" {
		attrs = append(attrs, "proxy_host", resolution.Host)
	}
	if resolution.Port > 0 {
		attrs = append(attrs, "proxy_port", resolution.Port)
	}
	slog.Info("kiro.proxy_resolved", attrs...)
}

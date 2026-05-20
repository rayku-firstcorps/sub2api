package service

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log/slog"
	"net/url"
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
		return kiroProxyResolution{}, err
	}
	logKiroProxyResolution(account, operation, resolution)
	return resolution, nil
}

func resolveKiroAccountProxy(account *Account) (kiroProxyResolution, error) {
	if account == nil {
		return kiroProxyResolution{Source: "none", Fingerprint: kiroProxyFingerprintNone}, nil
	}
	if account.ProxyID != nil {
		if account.Proxy == nil {
			return kiroProxyResolution{}, fmt.Errorf("kiro account %d has proxy_id %d but proxy is not loaded", account.ID, *account.ProxyID)
		}
		proxyURL := strings.TrimSpace(account.Proxy.URL())
		return kiroProxyResolution{
			URL:         proxyURL,
			Enabled:     proxyURL != "",
			Source:      "account_proxy",
			ProxyID:     *account.ProxyID,
			HasProxyID:  true,
			Protocol:    account.Proxy.Protocol,
			Host:        account.Proxy.Host,
			Port:        account.Proxy.Port,
			Fingerprint: kiroProxyFingerprintForBoundProxy(*account.ProxyID, proxyURL),
		}, nil
	}
	proxyURL := strings.TrimSpace(account.GetCredential("proxy_url"))
	resolution := kiroProxyResolution{
		URL:         proxyURL,
		Enabled:     proxyURL != "",
		Source:      "none",
		Fingerprint: kiroProxyFingerprintNone,
	}
	if proxyURL == "" {
		return resolution, nil
	}
	resolution.Source = "credential_proxy_url"
	resolution.Fingerprint = kiroProxyFingerprintForLegacyURL(proxyURL)
	if parsed, err := url.Parse(proxyURL); err == nil && parsed != nil {
		resolution.Protocol = parsed.Scheme
		resolution.Host = parsed.Hostname()
		if portText := parsed.Port(); portText != "" {
			if port, convErr := strconv.Atoi(portText); convErr == nil {
				resolution.Port = port
			}
		}
	}
	return resolution, nil
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

func kiroProxyFingerprintForLegacyURL(proxyURL string) string {
	return "credential_proxy_url:" + kiroProxyURLHash(proxyURL)
}

func kiroProxyURLHash(proxyURL string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(proxyURL)))
	return hex.EncodeToString(sum[:])[:16]
}

func logKiroProxyResolution(account *Account, operation string, resolution kiroProxyResolution) {
	operation = strings.TrimSpace(operation)
	if operation == "" {
		operation = "unknown"
	}
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

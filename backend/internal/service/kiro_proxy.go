package service

import (
	"fmt"
	"log/slog"
	"net/url"
	"strconv"
	"strings"
)

type kiroProxyResolution struct {
	URL        string
	Enabled    bool
	Source     string
	ProxyID    int64
	HasProxyID bool
	Protocol   string
	Host       string
	Port       int
}

func kiroAccountProxyURL(account *Account) (string, error) {
	resolution, err := resolveKiroAccountProxy(account)
	if err != nil {
		return "", err
	}
	return resolution.URL, nil
}

func kiroAccountProxyURLForOperation(account *Account, operation string) (string, error) {
	resolution, err := resolveKiroAccountProxy(account)
	if err != nil {
		return "", err
	}
	logKiroProxyResolution(account, operation, resolution)
	return resolution.URL, nil
}

func resolveKiroAccountProxy(account *Account) (kiroProxyResolution, error) {
	if account == nil {
		return kiroProxyResolution{Source: "none"}, nil
	}
	if account.ProxyID != nil {
		if account.Proxy == nil {
			return kiroProxyResolution{}, fmt.Errorf("kiro account %d has proxy_id %d but proxy is not loaded", account.ID, *account.ProxyID)
		}
		proxyURL := strings.TrimSpace(account.Proxy.URL())
		return kiroProxyResolution{
			URL:        proxyURL,
			Enabled:    proxyURL != "",
			Source:     "account_proxy",
			ProxyID:    *account.ProxyID,
			HasProxyID: true,
			Protocol:   account.Proxy.Protocol,
			Host:       account.Proxy.Host,
			Port:       account.Proxy.Port,
		}, nil
	}
	proxyURL := strings.TrimSpace(account.GetCredential("proxy_url"))
	resolution := kiroProxyResolution{
		URL:     proxyURL,
		Enabled: proxyURL != "",
		Source:  "none",
	}
	if proxyURL == "" {
		return resolution, nil
	}
	resolution.Source = "credential_proxy_url"
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

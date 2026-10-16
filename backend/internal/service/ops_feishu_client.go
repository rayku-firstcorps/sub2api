package service

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var feishuWebhookPath = regexp.MustCompile(`^/open-apis/bot/v2/hook/[A-Za-z0-9_-]{16,128}$`)

func validateFeishuWebhook(raw string) error {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.Host != "open.feishu.cn" || u.User != nil ||
		u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || u.RawPath != "" || !feishuWebhookPath.MatchString(u.Path) {
		return errors.New("Invalid Feishu webhook URL")
	}
	return nil
}

func newFeishuHTTPClient() *http.Client {
	// Dedicated transport: never inherit proxy settings or follow a redirect to an
	// arbitrary host. Validate resolved addresses again at connection time.
	transport := &http.Transport{TLSHandshakeTimeout: 5 * time.Second, ResponseHeaderTimeout: 8 * time.Second,
		IdleConnTimeout: 60 * time.Second, MaxIdleConnsPerHost: 2}
	transport.DialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(addr)
		if err != nil || host != "open.feishu.cn" || port != "443" {
			return nil, errors.New("Invalid Feishu destination")
		}
		ips, err := net.DefaultResolver.LookupIPAddr(ctx, host)
		if err != nil {
			return nil, errors.New("Feishu DNS lookup failed")
		}
		var dialer net.Dialer
		for _, ip := range ips {
			if !ip.IP.IsGlobalUnicast() || ip.IP.IsPrivate() || ip.IP.IsLoopback() || ip.IP.IsLinkLocalUnicast() {
				continue
			}
			conn, err := dialer.DialContext(ctx, network, net.JoinHostPort(ip.IP.String(), port))
			if err == nil {
				return conn, nil
			}
		}
		return nil, errors.New("Unable to connect to Feishu")
	}
	return &http.Client{Timeout: 10 * time.Second, Transport: transport,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
}

// Feishu signs an empty message with timestamp + "\n" + secret as the HMAC key.
func signFeishu(timestamp int64, secret string) string {
	mac := hmac.New(sha256.New, []byte(strconv.FormatInt(timestamp, 10)+"\n"+secret))
	return base64.StdEncoding.EncodeToString(mac.Sum(nil))
}

type feishuSendError struct {
	message   string
	retryable bool
}

func (e *feishuSendError) Error() string { return e.message }

func (s *OpsFeishuService) send(ctx context.Context, cfg *opsFeishuStoredConfig, message string) error {
	webhook, secret, err := s.credentials(cfg)
	if err != nil {
		return &feishuSendError{message: err.Error()}
	}
	now := time.Now().Unix()
	payload := struct {
		Timestamp string            `json:"timestamp"`
		Sign      string            `json:"sign"`
		MsgType   string            `json:"msg_type"`
		Content   map[string]string `json:"content"`
	}{strconv.FormatInt(now, 10), signFeishu(now, secret), "text", map[string]string{"text": message}}
	body, err := json.Marshal(payload)
	if err != nil || len(body) > 20*1024 {
		return &feishuSendError{message: "Feishu message exceeds size limit"}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, webhook, bytes.NewReader(body))
	if err != nil {
		return &feishuSendError{message: "Invalid Feishu request"}
	}
	req.Header.Set("Content-Type", "application/json")
	res, err := s.client.Do(req)
	if err != nil {
		// url.Error contains the entire secret-bearing webhook. Never propagate it.
		return &feishuSendError{message: "Feishu network request failed or timed out", retryable: true}
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return &feishuSendError{message: fmt.Sprintf("Feishu HTTP status %d", res.StatusCode), retryable: res.StatusCode == 429 || res.StatusCode >= 500}
	}
	raw, err := io.ReadAll(io.LimitReader(res.Body, 64*1024+1))
	if err != nil || len(raw) > 64*1024 {
		return &feishuSendError{message: "Invalid Feishu response", retryable: true}
	}
	var result struct {
		Code *int `json:"code"`
	}
	if json.Unmarshal(raw, &result) != nil || result.Code == nil {
		return &feishuSendError{message: "Invalid Feishu response", retryable: true}
	}
	if *result.Code != 0 {
		// Do not echo upstream msg fields: they may contain user input or credentials.
		return &feishuSendError{message: fmt.Sprintf("Feishu rejected message (code %d)", *result.Code), retryable: *result.Code == 11232}
	}
	return nil
}

func buildFeishuAlertMessage(event *OpsAlertEvent, kind string) string {
	label := "告警触发"
	if kind == OpsAlertStatusResolved {
		label = "告警恢复"
	}
	value, threshold := "-", "-"
	if event.MetricValue != nil {
		value = strconv.FormatFloat(*event.MetricValue, 'f', -1, 64)
	}
	if event.ThresholdValue != nil {
		threshold = strconv.FormatFloat(*event.ThresholdValue, 'f', -1, 64)
	}
	message := fmt.Sprintf("[Sub2API] %s · %s\n事件 #%d / 规则 #%d\n%s\n触发值: %s / 阈值: %s\n触发时间: %s",
		label, event.Severity, event.ID, event.RuleID, truncateString(event.Title, 512), value, threshold, event.FiredAt.UTC().Format(time.RFC3339))
	if event.ResolvedAt != nil && kind == OpsAlertStatusResolved {
		message += "\n恢复时间: " + event.ResolvedAt.UTC().Format(time.RFC3339)
	}
	// Only include known dimensions, never arbitrary payloads or credentials.
	for _, key := range []string{"platform", "group_id", "region"} {
		if v, ok := event.Dimensions[key]; ok {
			message += "\n" + key + ": " + truncateString(fmt.Sprint(v), 128)
		}
	}
	return strings.ToValidUTF8(message, "�")
}

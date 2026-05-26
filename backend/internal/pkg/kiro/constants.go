package kiro

import "fmt"

const (
	// AuthServiceEndpoint is the Kiro auth service for Social Auth token refresh.
	AuthServiceEndpoint = "https://prod.%s.auth.desktop.kiro.dev/refreshToken"

	// SSOOIDCEndpoint is the AWS SSO OIDC endpoint for Builder ID token refresh.
	SSOOIDCEndpoint = "https://oidc.%s.amazonaws.com/token"

	// GenerateAssistantResponseURL is the Kiro API endpoint.
	GenerateAssistantResponseURL = "https://q.%s.amazonaws.com/generateAssistantResponse"

	// UsageLimitsURL is the Kiro usage limits endpoint.
	UsageLimitsURL = "https://q.%s.amazonaws.com/getUsageLimits"

	DefaultRegion = "us-east-1"

	KiroVersion    = "0.11.63"
	UserAgentSDK   = "aws-sdk-js/1.0.34"
	APIName        = "codewhispererstreaming"
	APIVersion     = "1.0.34"
	MaxToolNameLen = 64
	MaxToolDescLen = 9216

	MaxRecentImagesMessages = 5

	AuthMethodSocial    = "social"
	AuthMethodBuilderID = "builder_id"

	ThinkingMinBudget     = 1024
	ThinkingMaxBudget     = 24576
	ThinkingDefaultBudget = 20000

	ChatTriggerTypeManual = "MANUAL"
	OriginAIEditor        = "AI_EDITOR"
	AgentTaskTypeVibe     = "vibe"
)

var DefaultModelMapping = map[string]string{
	"claude-haiku-4-5":           "claude-haiku-4.5",
	"claude-haiku-4-5-20251001":  "claude-haiku-4.5",
	"claude-sonnet-4-5":          "claude-sonnet-4.5",
	"claude-sonnet-4-5-20250929": "claude-sonnet-4.5",
	"claude-sonnet-4-6":          "claude-sonnet-4.6",
	"claude-opus-4-5":            "claude-opus-4.5",
	"claude-opus-4-5-20251101":   "claude-opus-4.5",
	"claude-opus-4-6":            "claude-opus-4.6",
	"claude-opus-4-7":            "claude-opus-4.7",
}

func SupportedModels() []string {
	seen := make(map[string]struct{})
	var models []string
	for k := range DefaultModelMapping {
		if _, ok := seen[k]; !ok {
			models = append(models, k)
			seen[k] = struct{}{}
		}
	}
	return models
}

func MapModel(model string) (string, bool) {
	mapped, ok := DefaultModelMapping[model]
	return mapped, ok
}

func GetRefreshURL(authMethod, region string) string {
	if region == "" {
		region = DefaultRegion
	}
	if authMethod == AuthMethodBuilderID {
		return fmt.Sprintf(SSOOIDCEndpoint, region)
	}
	return fmt.Sprintf(AuthServiceEndpoint, region)
}

func GetGenerateURL(region string) string {
	if region == "" {
		region = DefaultRegion
	}
	return fmt.Sprintf(GenerateAssistantResponseURL, region)
}

func GetUsageLimitsURL(region string) string {
	if region == "" {
		region = DefaultRegion
	}
	return fmt.Sprintf(UsageLimitsURL, region)
}

package kiro

type GenerateRequest struct {
	ConversationState ConversationState `json:"conversationState"`
	ProfileArn        string            `json:"profileArn,omitempty"`
}

type ConversationState struct {
	AgentTaskType   string `json:"agentTaskType"`
	ChatTriggerType string `json:"chatTriggerType"`
	ConversationID  string `json:"conversationId"`
	History         []Turn `json:"history,omitempty"`
	CurrentMessage  *Turn  `json:"currentMessage"`
}

type Turn struct {
	UserInputMessage         *UserInputMessage         `json:"userInputMessage,omitempty"`
	AssistantResponseMessage *AssistantResponseMessage `json:"assistantResponseMessage,omitempty"`
}

type UserInputMessage struct {
	Content                 string                   `json:"content"`
	ModelID                 string                   `json:"modelId"`
	Origin                  string                   `json:"origin"`
	Images                  []Image                  `json:"images,omitempty"`
	UserInputMessageContext *UserInputMessageContext `json:"userInputMessageContext,omitempty"`
}

type Image struct {
	Format string      `json:"format"`
	Source ImageSource `json:"source"`
}

type ImageSource struct {
	Bytes string `json:"bytes"`
}

type UserInputMessageContext struct {
	ToolResults []ToolResult      `json:"toolResults,omitempty"`
	Tools       []ToolSpecWrapper `json:"tools,omitempty"`
}

type ToolResult struct {
	ToolUseID string              `json:"toolUseId"`
	Content   []ToolResultContent `json:"content"`
	Status    string              `json:"status"`
}

type ToolResultContent struct {
	Text string `json:"text"`
}

type ToolSpecWrapper struct {
	ToolSpecification ToolSpecification `json:"toolSpecification"`
}

type ToolSpecification struct {
	Name        string      `json:"name"`
	Description string      `json:"description"`
	InputSchema InputSchema `json:"inputSchema"`
}

type InputSchema struct {
	JSON any `json:"json"`
}

type AssistantResponseMessage struct {
	Content  string    `json:"content"`
	ToolUses []ToolUse `json:"toolUses,omitempty"`
}

type ToolUse struct {
	ToolUseID string `json:"toolUseId"`
	Name      string `json:"name"`
	Input     any    `json:"input,omitempty"`
}

type StreamEvent struct {
	Content                string   `json:"content,omitempty"`
	Name                   string   `json:"name,omitempty"`
	ToolUseID              string   `json:"toolUseId,omitempty"`
	Input                  JSONText `json:"input,omitempty"`
	Stop                   *bool    `json:"stop,omitempty"`
	ContextUsagePercentage *int     `json:"contextUsagePercentage,omitempty"`
	Error                  string   `json:"error,omitempty"`
	ErrorCode              string   `json:"errorCode,omitempty"`
	ErrorMessage           string   `json:"errorMessage,omitempty"`
	FollowupPrompt         string   `json:"followupPrompt,omitempty"`
}

type SocialRefreshRequest struct {
	RefreshToken string `json:"refreshToken"`
}

type SocialRefreshResponse struct {
	AccessToken  string `json:"accessToken"`
	RefreshToken string `json:"refreshToken"`
	ProfileArn   string `json:"profileArn,omitempty"`
	ExpiresIn    int64  `json:"expiresIn"`
}

type BuilderIDRefreshRequest struct {
	ClientID     string `json:"clientId"`
	ClientSecret string `json:"clientSecret"`
	RefreshToken string `json:"refreshToken"`
	GrantType    string `json:"grantType"`
}

type BuilderIDRefreshResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token,omitempty"`
	ExpiresIn    int64  `json:"expires_in"`
	TokenType    string `json:"token_type,omitempty"`
}

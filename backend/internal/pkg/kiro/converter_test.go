package kiro

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestConvertRequestFiltersWebSearchTool(t *testing.T) {
	body := []byte(`{
		"model": "claude-sonnet-4-5",
		"messages": [{"role":"user","content":"weather in Zhuhai"}],
		"tools": [
			{"type":"web_search_20250305","name":"web_search"},
			{"name":"get_weather","description":"Get weather","input_schema":{"type":"object","properties":{"city":{"type":"string"}}}}
		]
	}`)

	req, err := ConvertRequest(body, "claude-sonnet-4.5", "")

	require.NoError(t, err)
	require.NotNil(t, req.ConversationState.CurrentMessage)
	ctx := req.ConversationState.CurrentMessage.UserInputMessage.UserInputMessageContext
	require.NotNil(t, ctx)
	require.Len(t, ctx.Tools, 1)
	require.Equal(t, "get_weather", ctx.Tools[0].ToolSpecification.Name)

	data, err := json.Marshal(req)
	require.NoError(t, err)
	require.NotContains(t, string(data), "web_search_20250305")
	require.NotContains(t, string(data), `"web_search"`)
}

func TestConvertRequestOmitsToolsWhenOnlyWebSearchTool(t *testing.T) {
	body := []byte(`{
		"model": "claude-sonnet-4-5",
		"messages": [{"role":"user","content":"hi"}],
		"tools": [
			{"type":"web_search_20250305","name":"web_search"}
		]
	}`)

	req, err := ConvertRequest(body, "claude-sonnet-4.5", "")

	require.NoError(t, err)
	require.NotNil(t, req.ConversationState.CurrentMessage)
	ctx := req.ConversationState.CurrentMessage.UserInputMessage.UserInputMessageContext
	require.Nil(t, ctx)
}

func TestConvertRequestOmitsToolsWhenNoTools(t *testing.T) {
	body := []byte(`{
		"model": "claude-sonnet-4-5",
		"messages": [{"role":"user","content":"hi"}]
	}`)

	req, err := ConvertRequest(body, "claude-sonnet-4.5", "")

	require.NoError(t, err)
	require.NotNil(t, req.ConversationState.CurrentMessage)
	ctx := req.ConversationState.CurrentMessage.UserInputMessage.UserInputMessageContext
	require.Nil(t, ctx)
}

func TestConvertRequestOmitsToolsWhenToolDescriptionsAreEmpty(t *testing.T) {
	body := []byte(`{
		"model": "claude-sonnet-4-5",
		"messages": [{"role":"user","content":"hi"}],
		"tools": [
			{"name":"bad_empty_description","description":"","input_schema":{"type":"object"}},
			{"name":"bad_blank_description","description":"   ","input_schema":{"type":"object"}}
		]
	}`)

	req, err := ConvertRequest(body, "claude-sonnet-4.5", "")

	require.NoError(t, err)
	require.NotNil(t, req.ConversationState.CurrentMessage)
	ctx := req.ConversationState.CurrentMessage.UserInputMessage.UserInputMessageContext
	require.Nil(t, ctx)
}

func TestConvertRequestAllowsToolWithoutInputSchema(t *testing.T) {
	body := []byte(`{
		"model": "claude-sonnet-4-5",
		"messages": [{"role":"user","content":"hi"}],
		"tools": [
			{"name":"ok_no_schema","description":"No schema"},
			{"name":"bad_array_schema","description":"Array schema","input_schema":[]}
		]
	}`)

	req, err := ConvertRequest(body, "claude-sonnet-4.5", "")

	require.NoError(t, err)
	require.NotNil(t, req.ConversationState.CurrentMessage)
	ctx := req.ConversationState.CurrentMessage.UserInputMessage.UserInputMessageContext
	require.NotNil(t, ctx)
	require.Len(t, ctx.Tools, 1)
	require.Equal(t, "ok_no_schema", ctx.Tools[0].ToolSpecification.Name)
}

func TestConvertRequestToolResultCurrentMessageUsesKiroShape(t *testing.T) {
	body := []byte(`{
		"model": "claude-sonnet-4-5",
		"messages": [
			{"role":"user","content":"call tool"},
			{"role":"assistant","content":[{"type":"tool_use","id":"toolu_1","name":"read_file","input":{"path":"README.md"}}]},
			{"role":"user","content":[{"type":"tool_result","tool_use_id":"toolu_1","content":[{"type":"text","text":"file contents"}]}]}
		],
		"tools": [
			{"name":"read_file","description":"Read a file","input_schema":{"type":"object","properties":{"path":{"type":"string"}}}}
		]
	}`)

	req, err := ConvertRequest(body, "claude-sonnet-4.5", "")

	require.NoError(t, err)
	require.NotNil(t, req.ConversationState.CurrentMessage)
	current := req.ConversationState.CurrentMessage.UserInputMessage
	require.Equal(t, "Tool results provided.", current.Content)
	require.NotNil(t, current.UserInputMessageContext)
	require.Len(t, current.UserInputMessageContext.ToolResults, 1)
	require.Equal(t, "toolu_1", current.UserInputMessageContext.ToolResults[0].ToolUseID)
	require.Equal(t, "file contents", current.UserInputMessageContext.ToolResults[0].Content[0].Text)
	require.Len(t, current.UserInputMessageContext.Tools, 1)

	data, err := json.Marshal(req)
	require.NoError(t, err)
	require.Contains(t, string(data), `"toolUses":[{"toolUseId":"toolu_1","name":"read_file","input":{"path":"README.md"}}]`)
	require.Contains(t, string(data), `"content":[{"text":"file contents"}]`)
}

func TestConvertRequestSanitizesAssistantToolUseInputLikeKiroClient(t *testing.T) {
	body := []byte(`{
		"model": "claude-sonnet-4-5",
		"messages": [
			{"role":"user","content":"call tool"},
			{"role":"assistant","content":[{"type":"tool_use","id":"toolu_1","name":"read_file","input":{"":"bad","path":"README.md"}}]}
		],
		"tools": [
			{"name":"read_file","description":"Read a file","input_schema":{"type":"object","properties":{"path":{"type":"string"}}}}
		]
	}`)

	req, err := ConvertRequest(body, "claude-sonnet-4.5", "")

	require.NoError(t, err)
	require.NotEmpty(t, req.ConversationState.History)
	toolUses := req.ConversationState.History[len(req.ConversationState.History)-1].AssistantResponseMessage.ToolUses
	require.Len(t, toolUses, 1)
	require.Equal(t, map[string]any{"path": "README.md"}, toolUses[0].Input)

	data, err := json.Marshal(req)
	require.NoError(t, err)
	require.Contains(t, string(data), `"input":{"path":"README.md"}`)
	require.NotContains(t, string(data), `"input":"{\"`)
	require.NotContains(t, string(data), `"":"bad"`)
}

func TestConvertRequestNormalizesComplexToolSchemaForKiro(t *testing.T) {
	body := []byte(`{
		"model": "claude-sonnet-4-5",
		"messages": [{"role":"user","content":"hi"}],
		"tools": [{
			"name":"complex_tool",
			"description":"Complex schema",
			"input_schema":{
				"$schema":"http://json-schema.org/draft-07/schema#",
				"$defs":{"pathDef":{"type":["string","null"],"minLength":1}},
				"type":"object",
				"properties":{
					"path":{"$ref":"#/$defs/pathDef"},
					"mode":{"enum":["read", 2, null]},
					"options":{"anyOf":[{"type":"object","properties":{"recursive":{"type":"boolean"}}},{"type":"string"}]},
					"tuple":{"type":"array","items":[{"type":"string"},{"type":"number"}]}
				},
				"required":["path","missing"],
				"additionalProperties":false
			}
		}]
	}`)

	req, err := ConvertRequest(body, "claude-sonnet-4.5", "")

	require.NoError(t, err)
	ctx := req.ConversationState.CurrentMessage.UserInputMessage.UserInputMessageContext
	require.NotNil(t, ctx)
	require.Len(t, ctx.Tools, 1)
	schema, ok := ctx.Tools[0].ToolSpecification.InputSchema.JSON.(map[string]any)
	require.True(t, ok)
	require.Equal(t, "object", schema["type"])
	require.NotContains(t, schema, "$schema")
	require.NotContains(t, schema, "$defs")
	require.NotContains(t, schema, "additionalProperties")
	require.Equal(t, []any{"path"}, schema["required"])

	props := schema["properties"].(map[string]any)
	path := props["path"].(map[string]any)
	require.Equal(t, "string", path["type"])
	require.Contains(t, path["description"], "nullable")
	require.Contains(t, path["description"], "minLen")

	mode := props["mode"].(map[string]any)
	require.Equal(t, "string", mode["type"])
	require.Equal(t, []any{"read", "2", "null"}, mode["enum"])

	options := props["options"].(map[string]any)
	require.Equal(t, "object", options["type"])
	require.NotContains(t, options, "anyOf")
	require.Contains(t, options["properties"], "recursive")

	tuple := props["tuple"].(map[string]any)
	require.Equal(t, "array", tuple["type"])
	require.Equal(t, "string", tuple["items"].(map[string]any)["type"])
}

func TestConvertRequestSupportsCustomToolShape(t *testing.T) {
	body := []byte(`{
		"model": "claude-sonnet-4-5",
		"messages": [{"role":"user","content":"hi"}],
		"tools": [{
			"type":"custom",
			"name":"custom_exec",
			"custom":{
				"description":"Execute custom work",
				"input_schema":{"properties":{"cmd":{"type":"string"}}}
			}
		}]
	}`)

	req, err := ConvertRequest(body, "claude-sonnet-4.5", "")

	require.NoError(t, err)
	ctx := req.ConversationState.CurrentMessage.UserInputMessage.UserInputMessageContext
	require.NotNil(t, ctx)
	require.Len(t, ctx.Tools, 1)
	tool := ctx.Tools[0].ToolSpecification
	require.Equal(t, "custom_exec", tool.Name)
	require.Equal(t, "Execute custom work", tool.Description)
	schema := tool.InputSchema.JSON.(map[string]any)
	require.Equal(t, "object", schema["type"])
	require.Contains(t, schema["properties"], "cmd")
}

func TestConvertRequestClampsSmallThinkingBudgetToMinimum(t *testing.T) {
	body := []byte(`{
		"model": "claude-sonnet-4-5",
		"thinking": {"type":"enabled","budget_tokens":512},
		"system": "system prompt",
		"messages": [{"role":"user","content":"hi"}]
	}`)

	req, err := ConvertRequest(body, "claude-sonnet-4.5", "")

	require.NoError(t, err)
	current := req.ConversationState.CurrentMessage.UserInputMessage
	require.Contains(t, current.Content, "<max_thinking_length>1024</max_thinking_length>")
	require.Contains(t, current.Content, "system prompt")
}

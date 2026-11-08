package chat

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/pdacton/visoto/internal/logger"
)

// The Anthropic provider: used when the caller brings their own Anthropic API
// key (commercial edition, UA-50). Same prompt, same MCP tools and the same
// iteration cap as the Gemini path.
//
// A small client over the Messages API rather than the official Go SDK: the
// chat needs one endpoint, and the SDK's generated package needs ~2.3 GB of
// RAM to compile, which stalled image builds on the production host. Keep
// this to the documented wire format (https://docs.anthropic.com/en/api/messages).

const (
	anthropicModel   = "claude-opus-5-5"
	anthropicVersion = "2023-06-01"
	// A refusal is re-served by a model the API picks per refusal category.
	anthropicFallbackBeta = "server-side-fallback-2026-07-01"
	anthropicTimeout      = 120 * time.Second
	anthropicMaxBody      = 8 << 20
)

// anthropicBaseURL is api.anthropic.com; tests point it at a fake.
var anthropicBaseURL = "https://api.anthropic.com"

var anthropicHTTP = &http.Client{Timeout: anthropicTimeout}

// ErrInvalidKey reports that the provider rejected the caller's API key.
var ErrInvalidKey = errors.New("the AI provider rejected the API key")

// ---- wire format ----

type anthropicTool struct {
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	InputSchema map[string]any `json:"input_schema"`
}

// anthropicMessage is one turn. Content is kept as raw blocks so an assistant
// turn goes back exactly as it came (thinking blocks included): the
// conversation stays append-only.
type anthropicMessage struct {
	Role    string            `json:"role"`
	Content []json.RawMessage `json:"content"`
}

type anthropicRequest struct {
	Model        string             `json:"model"`
	MaxTokens    int                `json:"max_tokens"`
	System       string             `json:"system,omitempty"`
	Messages     []anthropicMessage `json:"messages"`
	Tools        []anthropicTool    `json:"tools,omitempty"`
	OutputConfig map[string]any     `json:"output_config,omitempty"`
	Fallbacks    string             `json:"fallbacks,omitempty"`
}

type anthropicResponse struct {
	Content    []json.RawMessage `json:"content"`
	StopReason string            `json:"stop_reason"`
}

// anthropicBlock is the part of a content block the loop reads.
type anthropicBlock struct {
	Type  string          `json:"type"`
	Text  string          `json:"text"`
	ID    string          `json:"id"`
	Name  string          `json:"name"`
	Input json.RawMessage `json:"input"`
}

func textBlock(s string) json.RawMessage {
	b, _ := json.Marshal(map[string]string{"type": "text", "text": s})
	return b
}

func toolResultBlock(id, content string, isError bool) json.RawMessage {
	b, _ := json.Marshal(map[string]any{"type": "tool_result", "tool_use_id": id, "content": content, "is_error": isError})
	return b
}

// ---- the loop ----

// CallAnthropic answers req with Claude, calling Visoto's MCP tools as needed.
func CallAnthropic(ctx context.Context, apiKey, mcpURL string, req ChatRequest) (string, error) {
	dataJSON, err := marshalData(req.Data)
	if err != nil {
		return "", err
	}
	var messages []anthropicMessage
	for _, m := range req.History {
		if m.Content == "" {
			continue
		}
		role := "user"
		if m.Role == "assistant" {
			role = "assistant"
		}
		messages = append(messages, anthropicMessage{Role: role, Content: []json.RawMessage{textBlock(m.Content)}})
	}
	messages = append(messages, anthropicMessage{Role: "user", Content: []json.RawMessage{textBlock(req.Message)}})

	body := anthropicRequest{
		Model:        anthropicModel,
		MaxTokens:    16000,
		System:       buildSystemPrompt(req.AcceptLanguage, req.endpointName, dataJSON),
		Tools:        anthropicTools(),
		OutputConfig: map[string]any{"effort": "medium"},
		Fallbacks:    "default",
	}

	log := logger.Get()
	for i := range maxAgenticIterations {
		body.Messages = messages
		resp, err := postAnthropic(ctx, apiKey, body)
		if err != nil {
			return "", err
		}
		if resp.StopReason == "refusal" {
			return "", errors.New("the AI provider declined to answer this request")
		}
		messages = append(messages, anthropicMessage{Role: "assistant", Content: resp.Content})

		var text strings.Builder
		var results []json.RawMessage
		for _, raw := range resp.Content {
			var b anthropicBlock
			if err := json.Unmarshal(raw, &b); err != nil {
				continue
			}
			switch b.Type {
			case "text":
				text.WriteString(b.Text)
			case "tool_use":
				var args map[string]any
				_ = json.Unmarshal(b.Input, &args)
				log.Debug("calling MCP tool", slog.String("tool", b.Name), slog.String("provider", "anthropic"))
				content, isError := toolResultText(callMCPTool(ctx, mcpURL, b.Name, args))
				results = append(results, toolResultBlock(b.ID, content, isError))
			}
		}
		if resp.StopReason != "tool_use" || len(results) == 0 {
			if text.Len() == 0 {
				return "", fmt.Errorf("empty response from API after %d iteration(s)", i+1)
			}
			return transformIRIsToLinks(convertHTMLLinksToMarkdown(text.String())), nil
		}
		// All tool results of a turn go back in one user message.
		messages = append(messages, anthropicMessage{Role: "user", Content: results})
	}
	return "", fmt.Errorf("agentic loop exceeded %d iterations without a final response", maxAgenticIterations)
}

// postAnthropic sends one Messages request, retrying once on an overloaded or
// failing server. Only the caller's key is sent — never a server credential.
func postAnthropic(ctx context.Context, apiKey string, body anthropicRequest) (anthropicResponse, error) {
	payload, err := json.Marshal(body)
	if err != nil {
		return anthropicResponse{}, err
	}
	for attempt := 0; ; attempt++ {
		httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(anthropicBaseURL, "/")+"/v1/messages", bytes.NewReader(payload))
		if err != nil {
			return anthropicResponse{}, err
		}
		httpReq.Header.Set("Content-Type", "application/json")
		httpReq.Header.Set("X-Api-Key", apiKey)
		httpReq.Header.Set("Anthropic-Version", anthropicVersion)
		httpReq.Header.Set("Anthropic-Beta", anthropicFallbackBeta)
		resp, err := anthropicHTTP.Do(httpReq)
		if err != nil {
			return anthropicResponse{}, fmt.Errorf("anthropic: %w", err)
		}
		raw, err := io.ReadAll(io.LimitReader(resp.Body, anthropicMaxBody))
		resp.Body.Close()
		if err != nil {
			return anthropicResponse{}, fmt.Errorf("anthropic: %w", err)
		}
		switch {
		case resp.StatusCode == http.StatusOK:
			var out anthropicResponse
			if err := json.Unmarshal(raw, &out); err != nil {
				return anthropicResponse{}, fmt.Errorf("anthropic: bad response: %w", err)
			}
			return out, nil
		case resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden:
			return anthropicResponse{}, ErrInvalidKey
		case resp.StatusCode == http.StatusTooManyRequests:
			return anthropicResponse{}, fmt.Errorf("rate_limit: %s", "a moment")
		case resp.StatusCode >= 500 && attempt == 0:
			select {
			case <-ctx.Done():
				return anthropicResponse{}, ctx.Err()
			case <-time.After(time.Second):
			}
			continue
		default:
			return anthropicResponse{}, fmt.Errorf("anthropic: status %d: %s", resp.StatusCode, anthropicErrorMessage(raw))
		}
	}
}

// anthropicErrorMessage extracts error.message from an error body.
func anthropicErrorMessage(raw []byte) string {
	var e struct {
		Error struct {
			Type    string `json:"type"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if json.Unmarshal(raw, &e) == nil && e.Error.Message != "" {
		return e.Error.Type + ": " + e.Error.Message
	}
	if len(raw) > 300 {
		raw = raw[:300]
	}
	return string(raw)
}

// toolResultText flattens callMCPTool's map into tool_result content.
func toolResultText(out map[string]any) (string, bool) {
	if e, ok := out["error"].(string); ok {
		return e, true
	}
	s, _ := out["result"].(string)
	isErr, _ := out["isError"].(bool)
	if s == "" {
		s = "(no result)"
	}
	return s, isErr
}

// anthropicTools converts the Gemini function declarations — the single list
// of MCP tools the chat offers — into Anthropic tool definitions.
func anthropicTools() []anthropicTool {
	var out []anthropicTool
	for _, group := range buildMCPToolDeclarations() {
		for _, d := range group.FunctionDeclarations {
			props := map[string]any{}
			for name, p := range d.Parameters.Properties {
				prop := map[string]any{"type": p.Type}
				if p.Description != "" {
					prop["description"] = p.Description
				}
				props[name] = prop
			}
			schema := map[string]any{"type": "object", "properties": props}
			if len(d.Parameters.Required) > 0 {
				schema["required"] = d.Parameters.Required
			}
			out = append(out, anthropicTool{Name: d.Name, Description: d.Description, InputSchema: schema})
		}
	}
	return out
}

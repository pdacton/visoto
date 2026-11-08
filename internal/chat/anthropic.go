package chat

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/anthropics/anthropic-sdk-go/v2"
	"github.com/anthropics/anthropic-sdk-go/v2/option"
	"github.com/anthropics/anthropic-sdk-go/v2/shared/constant"
	"github.com/pdacton/visoto/internal/logger"
)

// The Anthropic provider: used when the caller brings their own Anthropic API
// key (commercial edition, UA-50). Same prompt, same MCP tools and the same
// iteration cap as the Gemini path.

const (
	anthropicModel   = "claude-opus-5-5"
	anthropicTimeout = 120 * time.Second
)

// anthropicBaseURL overrides api.anthropic.com, for tests only.
var anthropicBaseURL = ""

// ErrInvalidKey reports that the provider rejected the caller's API key.
var ErrInvalidKey = errors.New("the AI provider rejected the API key")

// CallAnthropic answers req with Claude, calling Visoto's MCP tools as needed.
func CallAnthropic(ctx context.Context, apiKey, mcpURL string, req ChatRequest) (string, error) {
	dataJSON, err := marshalData(req.Data)
	if err != nil {
		return "", err
	}
	// Only the caller's key: never ANTHROPIC_* variables or a CLI profile
	// that happen to exist on the server.
	opts := []option.RequestOption{option.WithoutEnvironmentDefaults(), option.WithAPIKey(apiKey), option.WithRequestTimeout(anthropicTimeout), option.WithMaxRetries(1)}
	if anthropicBaseURL != "" {
		opts = append(opts, option.WithBaseURL(anthropicBaseURL))
	}
	client := anthropic.NewClient(opts...)

	var messages []anthropic.BetaMessageParam
	for _, m := range req.History {
		if m.Content == "" {
			continue
		}
		if m.Role == "assistant" {
			messages = append(messages, anthropic.BetaMessageParam{OfAssistant: &anthropic.BetaAssistantMessageParam{
				Content: []anthropic.BetaContentBlockParamUnion{anthropic.NewBetaTextBlock(m.Content)},
			}})
		} else {
			messages = append(messages, anthropic.NewBetaUserMessage(anthropic.NewBetaTextBlock(m.Content)))
		}
	}
	messages = append(messages, anthropic.NewBetaUserMessage(anthropic.NewBetaTextBlock(req.Message)))

	params := anthropic.BetaMessageNewParams{
		Model:     anthropicModel,
		MaxTokens: 16000,
		System:    []anthropic.BetaTextBlockParam{{Text: buildSystemPrompt(req.AcceptLanguage, req.endpointName, dataJSON)}},
		Tools:     anthropicTools(),
		OutputConfig: anthropic.BetaOutputConfigParam{
			Effort: anthropic.BetaOutputConfigEffortMedium,
		},
		// A refusal is re-served by a model the API picks per refusal category.
		Fallbacks: anthropic.BetaFallbacksParamUnion{OfDefault: constant.ValueOf[constant.Default]()},
		Betas:     []anthropic.AnthropicBeta{anthropic.AnthropicBetaServerSideFallback2026_07_01},
	}

	log := logger.Get()
	for i := range maxAgenticIterations {
		params.Messages = messages
		resp, err := client.Beta.Messages.New(ctx, params)
		if err != nil {
			var apiErr *anthropic.Error
			if errors.As(err, &apiErr) {
				switch apiErr.StatusCode {
				case 401, 403:
					return "", ErrInvalidKey
				case 429:
					return "", fmt.Errorf("rate_limit: %s", "a moment")
				}
			}
			return "", fmt.Errorf("anthropic: %w", err)
		}
		if resp.StopReason == anthropic.BetaStopReasonRefusal {
			return "", errors.New("the AI provider declined to answer this request")
		}
		// The whole response goes back into the history, thinking blocks
		// included: the conversation stays append-only.
		messages = append(messages, resp.ToParam())

		var text strings.Builder
		var results []anthropic.BetaContentBlockParamUnion
		for _, block := range resp.Content {
			switch b := block.AsAny().(type) {
			case anthropic.BetaTextBlock:
				text.WriteString(b.Text)
			case anthropic.BetaToolUseBlock:
				var args map[string]any
				_ = json.Unmarshal([]byte(b.JSON.Input.Raw()), &args)
				log.Debug("calling MCP tool", slog.String("tool", b.Name), slog.String("provider", "anthropic"))
				out := callMCPTool(ctx, mcpURL, b.Name, args)
				content, isError := toolResultText(out)
				results = append(results, anthropic.NewBetaToolResultBlock(b.ID, content, isError))
			}
		}
		if resp.StopReason != anthropic.BetaStopReasonToolUse || len(results) == 0 {
			if text.Len() == 0 {
				return "", fmt.Errorf("empty response from API after %d iteration(s)", i+1)
			}
			return transformIRIsToLinks(convertHTMLLinksToMarkdown(text.String())), nil
		}
		messages = append(messages, anthropic.NewBetaUserMessage(results...))
	}
	return "", fmt.Errorf("agentic loop exceeded %d iterations without a final response", maxAgenticIterations)
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
func anthropicTools() []anthropic.BetaToolUnionParam {
	var out []anthropic.BetaToolUnionParam
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
			tool := anthropic.BetaToolParam{
				Name:        d.Name,
				Description: anthropic.String(d.Description),
				InputSchema: anthropic.BetaToolInputSchemaParam{Properties: props, Required: d.Parameters.Required},
			}
			out = append(out, anthropic.BetaToolUnionParam{OfTool: &tool})
		}
	}
	return out
}

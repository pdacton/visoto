package chat

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"regexp"
	"strings"
	"time"

	mcpClient "github.com/mark3labs/mcp-go/client"
	mcpTransport "github.com/mark3labs/mcp-go/client/transport"
	"github.com/mark3labs/mcp-go/mcp"
	"hutzli.org/visoto/internal/logger"
	"hutzli.org/visoto/internal/sparql"
)

const (
	geminiEndpoint      = "https://generativelanguage.googleapis.com/v1beta/models/gemini-3.1-flash-lite-preview:generateContent"
	geminiTimeout       = 30 * time.Second
	mcpCallTimeout      = 60 * time.Second
	maxAgenticIterations = 5
)

// CallGemini sends a request to the Gemini API with MCP tool support and returns the formatted response.
func CallGemini(apiKey, mcpURL string, req ChatRequest) (string, error) {
	// 1. Format SPARQL data as JSON for context
	buffer := new(bytes.Buffer)
	encoder := json.NewEncoder(buffer)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(req.Data); err != nil {
		return "", fmt.Errorf("failed to marshal data: %w", err)
	}
	dataJSON := bytes.TrimRight(buffer.Bytes(), "\n")

	// 2. Build system prompt and contents
	systemPrompt := buildSystemPrompt(req.AcceptLanguage, req.ActiveEndpoint, string(dataJSON))
	contents := buildContents(systemPrompt, req.History, req.Message)
	tools := buildMCPToolDeclarations()

	// 3. Run agentic loop (Gemini ↔ MCP tools)
	response, err := runAgenticLoop(context.Background(), apiKey, mcpURL, contents, tools)
	if err != nil {
		return "", err
	}

	// 4. Post-process: convert HTML links and bare IRIs to markdown
	response = convertHTMLLinksToMarkdown(response)
	response = transformIRIsToLinks(response)

	return response, nil
}

// buildSystemPrompt creates the system prompt with instructions and pre-fetched data.
func buildSystemPrompt(language string, endpoint ActiveEndpointInfo, dataJSON string) string {
	return fmt.Sprintf(`You are a helpful assistant answering questions about RDF resources.

ACTIVE SPARQL ENDPOINT: %s (%s)
When calling MCP tools that accept an "endpoint" parameter, always use this endpoint name unless the user explicitly asks otherwise.

RULES:
1. The RESOURCE DATA below is pre-fetched context about the current resource — it may have come from a different endpoint. Do NOT mention this discrepancy to the user.
2. When answering questions, always query the ACTIVE SPARQL ENDPOINT above using run_sparql_query. Use the RESOURCE DATA only as a hint for what IRIs or properties to look for.
3. If the answer is not in the provided data, you MUST call run_sparql_query (or another MCP tool) to fetch it from the active endpoint — do NOT say "I don't have enough information" without first trying a tool call.
4. Only after tool calls also fail to return the needed data, say: "I don't have enough information to answer that question."
5. When referring to other resources (URIs), format them ONLY as markdown links: [Label](URI) - NEVER use HTML tags
6. IMPORTANT: Use markdown syntax for links, NOT HTML. Write [text](url) not <a href="url">text</a>
7. Respond in the following language: %s
8. Be concise but informative - prefer 2-3 sentence answers unless more detail is requested
9. Use the "Lol" field value for display when available (it contains the human-readable label)

RESOURCE DATA (pre-fetched context, may be from a different endpoint — use as reference only):
%s`, endpoint.Name, endpoint.URL, language, dataJSON)
}

// buildContents creates the Gemini API contents array from system prompt, history, and current message.
func buildContents(systemPrompt string, history []Message, currentMsg string) []geminiContent {
	var contents []geminiContent

	// System prompt as first user turn
	contents = append(contents, geminiContent{
		Role:  "user",
		Parts: []geminiPart{{Text: systemPrompt}},
	})

	// Model acknowledgment
	contents = append(contents, geminiContent{
		Role:  "model",
		Parts: []geminiPart{{Text: "I understand. I will answer questions based on the provided SPARQL data and use the available tools to fetch additional information when needed."}},
	})

	// Conversation history
	for _, msg := range history {
		role := "user"
		if msg.Role == "assistant" {
			role = "model"
		}
		contents = append(contents, geminiContent{
			Role:  role,
			Parts: []geminiPart{{Text: msg.Content}},
		})
	}

	// Current user message
	contents = append(contents, geminiContent{
		Role:  "user",
		Parts: []geminiPart{{Text: currentMsg}},
	})

	return contents
}

// buildMCPToolDeclarations returns Gemini function declarations matching the Visoto MCP server tools.
func buildMCPToolDeclarations() []geminiTool {
	return []geminiTool{{
		FunctionDeclarations: []geminiFunctionDecl{
			{
				Name:        "list_endpoints",
				Description: "List all configured SPARQL triple store endpoints with their names and URLs.",
				Parameters:  geminiSchemaObject{Type: "object"},
			},
			{
				Name:        "list_prefixes",
				Description: "List all configured RDF prefix declarations (e.g. schema:, rdf:, dct:).",
				Parameters:  geminiSchemaObject{Type: "object"},
			},
			{
				Name:        "check_endpoint",
				Description: "Check if a SPARQL endpoint is reachable by running ASK {}. Returns status and latency.",
				Parameters: geminiSchemaObject{
					Type: "object",
					Properties: map[string]geminiSchemaProp{
						"endpoint": {Type: "string", Description: "Endpoint name (e.g. 'LINDAS prod') or full URL. Uses the default endpoint if omitted."},
					},
				},
			},
			{
				Name: "run_sparql_query",
				Description: "Execute a SPARQL SELECT query against a configured endpoint. " +
					"Configured RDF prefixes (rdf:, schema:, dct:, etc.) are injected automatically — no need to declare them. " +
					"Returns results as JSON rows. On failure, returns helpful hints.",
				Parameters: geminiSchemaObject{
					Type: "object",
					Properties: map[string]geminiSchemaProp{
						"query":          {Type: "string", Description: "SPARQL SELECT query text. Prefixes are injected automatically."},
						"endpoint":       {Type: "string", Description: "Endpoint name or full URL. Uses the default endpoint if omitted."},
						"resolve_labels": {Type: "boolean", Description: "If true, resolve IRIs to human-readable labels. Default: false."},
					},
					Required: []string{"query"},
				},
			},
			{
				Name:        "discover_classes",
				Description: "Discover the most common RDF types/classes in an endpoint, ordered by instance count.",
				Parameters: geminiSchemaObject{
					Type: "object",
					Properties: map[string]geminiSchemaProp{
						"endpoint": {Type: "string", Description: "Endpoint name or URL. Uses the default endpoint if omitted."},
						"limit":    {Type: "number", Description: "Maximum number of classes to return. Default: 100."},
					},
				},
			},
			{
				Name:        "discover_properties",
				Description: "Discover the most frequently used RDF predicates/properties in an endpoint.",
				Parameters: geminiSchemaObject{
					Type: "object",
					Properties: map[string]geminiSchemaProp{
						"endpoint": {Type: "string", Description: "Endpoint name or URL. Uses the default endpoint if omitted."},
						"limit":    {Type: "number", Description: "Maximum number of properties to return. Default: 100."},
					},
				},
			},
			{
				Name:        "get_resource",
				Description: "Retrieve all RDF triples (predicate–object pairs) for a given resource IRI.",
				Parameters: geminiSchemaObject{
					Type: "object",
					Properties: map[string]geminiSchemaProp{
						"iri":      {Type: "string", Description: "The full IRI of the resource to retrieve."},
						"endpoint": {Type: "string", Description: "Endpoint name or URL. Uses the default endpoint if omitted."},
					},
					Required: []string{"iri"},
				},
			},
			{
				Name:        "search_by_label",
				Description: "Search for RDF resources by label text using rdfs:label and skos:prefLabel. Case-insensitive substring match.",
				Parameters: geminiSchemaObject{
					Type: "object",
					Properties: map[string]geminiSchemaProp{
						"text":        {Type: "string", Description: "Search string to find in resource labels."},
						"type_filter": {Type: "string", Description: "Optional RDF type IRI to filter results, e.g. https://schema.org/Person."},
						"endpoint":    {Type: "string", Description: "Endpoint name or URL. Uses the default endpoint if omitted."},
						"limit":       {Type: "number", Description: "Maximum number of results. Default: 50."},
					},
					Required: []string{"text"},
				},
			},
			{
				Name:        "count_instances",
				Description: "Count RDF instances per class in the endpoint. Provide class_iri to count only instances of a specific class.",
				Parameters: geminiSchemaObject{
					Type: "object",
					Properties: map[string]geminiSchemaProp{
						"class_iri": {Type: "string", Description: "Optional RDF class IRI to count instances for. If omitted, counts for all classes."},
						"endpoint":  {Type: "string", Description: "Endpoint name or URL. Uses the default endpoint if omitted."},
					},
				},
			},
		},
	}}
}

// runAgenticLoop sends contents to Gemini and executes any requested tool calls,
// repeating until Gemini returns a final text response or the iteration limit is reached.
func runAgenticLoop(ctx context.Context, apiKey, mcpURL string, contents []geminiContent, tools []geminiTool) (string, error) {
	log := logger.Get()

	for i := range maxAgenticIterations {
		candidate, err := callGeminiAPIRaw(apiKey, contents, tools)
		if err != nil {
			return "", err
		}

		// Collect function calls from this turn
		var functionCalls []geminiFunctionCall
		for _, part := range candidate.Content.Parts {
			if part.FunctionCall != nil {
				functionCalls = append(functionCalls, *part.FunctionCall)
			}
		}

		// No function calls → this is the final text response
		if len(functionCalls) == 0 {
			// Skip thought parts; find the first real text part
			var text string
			for _, p := range candidate.Content.Parts {
				if !p.Thought && p.Text != "" {
					text = p.Text
					break
				}
			}
			if text == "" {
				return "", fmt.Errorf("empty response from API after %d iteration(s)", i+1)
			}
			log.Debug("agentic loop complete", slog.Int("iterations", i+1))
			return text, nil
		}

		log.Debug("gemini requested tool calls",
			slog.Int("iteration", i+1),
			slog.Int("toolCallCount", len(functionCalls)))

		// Append the model's function-call turn to history
		contents = append(contents, candidate.Content)

		// Execute each tool call and collect responses
		var responseParts []geminiPart
		for _, fc := range functionCalls {
			log.Debug("calling MCP tool", slog.String("tool", fc.Name))
			toolResult := callMCPTool(ctx, mcpURL, fc.Name, fc.Args)
			responseParts = append(responseParts, geminiPart{
				FunctionResponse: &geminiFunctionResponse{
					Name:     fc.Name,
					Response: toolResult,
				},
			})
		}

		// Append the function response turn
		contents = append(contents, geminiContent{
			Role:  "user",
			Parts: responseParts,
		})
	}

	return "", fmt.Errorf("agentic loop exceeded %d iterations without a final response", maxAgenticIterations)
}

// callMCPTool calls a single tool on the Visoto MCP server and returns the result as a map.
// Errors are returned as map values (never as Go errors) so Gemini can handle them gracefully.
func callMCPTool(ctx context.Context, mcpURL, toolName string, args map[string]any) map[string]any {
	trans, err := mcpTransport.NewStreamableHTTP(mcpURL)
	if err != nil {
		return map[string]any{"error": fmt.Sprintf("MCP transport error: %v", err)}
	}

	client := mcpClient.NewClient(trans)

	callCtx, cancel := context.WithTimeout(ctx, mcpCallTimeout)
	defer cancel()

	_, err = client.Initialize(callCtx, mcp.InitializeRequest{
		Params: mcp.InitializeParams{
			ClientInfo: mcp.Implementation{Name: "visoto-chat", Version: "1.0"},
		},
	})
	if err != nil {
		return map[string]any{"error": fmt.Sprintf("MCP init failed: %v", err)}
	}

	result, err := client.CallTool(callCtx, mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name:      toolName,
			Arguments: args,
		},
	})
	if err != nil {
		return map[string]any{"error": fmt.Sprintf("MCP tool call failed: %v", err)}
	}

	// Concatenate all text content parts
	var sb strings.Builder
	for _, c := range result.Content {
		if textContent, ok := c.(mcp.TextContent); ok {
			sb.WriteString(textContent.Text)
		}
	}
	return map[string]any{"result": sb.String(), "isError": result.IsError}
}

// callGeminiAPIRaw makes the HTTP request to the Gemini API and returns the first candidate.
func callGeminiAPIRaw(apiKey string, contents []geminiContent, tools []geminiTool) (geminiCandidate, error) {
	reqBody := geminiRequest{
		Contents: contents,
		Tools:    tools,
		ToolConfig: &geminiToolConfig{
			FunctionCallingConfig: geminiFunctionCallingConfig{Mode: "AUTO"},
		},
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return geminiCandidate{}, fmt.Errorf("failed to marshal request: %w", err)
	}

	url := fmt.Sprintf("%s?key=%s", geminiEndpoint, apiKey)
	httpReq, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return geminiCandidate{}, fmt.Errorf("failed to create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	httpClient := &http.Client{Timeout: geminiTimeout}
	resp, err := httpClient.Do(httpReq)
	if err != nil {
		return geminiCandidate{}, fmt.Errorf("API request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return geminiCandidate{}, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		if resp.StatusCode == http.StatusTooManyRequests {
			return geminiCandidate{}, fmt.Errorf("rate_limit: %s", extractRetryDelay(body))
		}
		return geminiCandidate{}, fmt.Errorf("API returned status %d: %s", resp.StatusCode, string(body))
	}

	var geminiResp geminiResponse
	if err := json.Unmarshal(body, &geminiResp); err != nil {
		return geminiCandidate{}, fmt.Errorf("failed to parse response: %w", err)
	}

	if len(geminiResp.Candidates) == 0 {
		return geminiCandidate{}, fmt.Errorf("no candidates in API response")
	}

	return geminiResp.Candidates[0], nil
}

// extractRetryDelay parses the retryDelay from a Gemini 429 response body.
// Returns a human-readable string like "2s" or a fallback message.
func extractRetryDelay(body []byte) string {
	// The retry delay appears as "retryDelay": "2s" or "retryDelay": "2.704704531s"
	re := regexp.MustCompile(`"retryDelay"\s*:\s*"([^"]+)"`)
	if m := re.FindSubmatch(body); len(m) == 2 {
		return string(m[1])
	}
	return "a moment"
}

// convertHTMLLinksToMarkdown converts HTML anchor tags to markdown format.
func convertHTMLLinksToMarkdown(text string) string {
	htmlLinkPattern := regexp.MustCompile(`<a\s+href="([^"]+)">([^<]+)</a>`)
	return htmlLinkPattern.ReplaceAllString(text, "[$2]($1)")
}

// transformIRIsToLinks converts bare URIs and markdown links to Visoto resource links.
func transformIRIsToLinks(text string) string {
	// Handle URIs already in markdown format: [text](https://...)
	markdownPattern := regexp.MustCompile(`\[([^\]]+)\]\((https?://[^)]+)\)`)
	result := markdownPattern.ReplaceAllStringFunc(text, func(match string) string {
		submatches := markdownPattern.FindStringSubmatch(match)
		if len(submatches) == 3 {
			label := submatches[1]
			uri := submatches[2]
			return fmt.Sprintf("[%s](%s)", label, sparql.ResourceHref(uri))
		}
		return match
	})

	// Handle bare URIs not inside markdown links
	bareUriPattern := regexp.MustCompile(`(^|[\s])(https?://[^\s)\]]+)([\s.,;!?]|$)`)
	result = bareUriPattern.ReplaceAllStringFunc(result, func(match string) string {
		submatches := bareUriPattern.FindStringSubmatch(match)
		if len(submatches) == 4 {
			prefix := submatches[1]
			uri := submatches[2]
			suffix := submatches[3]
			label := extractLabel(uri)
			return fmt.Sprintf("%s[%s](%s)%s", prefix, label, sparql.ResourceHref(uri), suffix)
		}
		return match
	})

	return result
}

// extractLabel extracts a human-readable label from a URI (last segment after / or #).
func extractLabel(uri string) string {
	uri = strings.TrimSpace(uri)
	lastSlash := strings.LastIndex(uri, "/")
	lastHash := strings.LastIndex(uri, "#")
	pos := lastSlash
	if lastHash > pos {
		pos = lastHash
	}
	if pos >= 0 && pos < len(uri)-1 {
		label := uri[pos+1:]
		label = strings.ReplaceAll(label, "%3A", ":")
		label = strings.ReplaceAll(label, "%20", " ")
		return label
	}
	return uri
}

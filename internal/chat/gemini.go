package chat

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"
)

const (
	geminiEndpoint = "https://generativelanguage.googleapis.com/v1beta/models/gemini-2.5-flash:generateContent"
	geminiTimeout  = 30 * time.Second
)

// Gemini API request/response structures
type geminiRequest struct {
	Contents []geminiContent `json:"contents"`
}

type geminiContent struct {
	Role  string        `json:"role"` // "user" or "model"
	Parts []geminiPart  `json:"parts"`
}

type geminiPart struct {
	Text string `json:"text"`
}

type geminiResponse struct {
	Candidates []geminiCandidate `json:"candidates"`
}

type geminiCandidate struct {
	Content geminiContent `json:"content"`
}

// CallGemini sends a request to the Gemini API and returns the formatted response
func CallGemini(apiKey string, req ChatRequest) (string, error) {
	// 1. Format SPARQL data as JSON for context
	buffer := new(bytes.Buffer)
	encoder := json.NewEncoder(buffer)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(req.Data); err != nil {
		return "", fmt.Errorf("failed to marshal data: %w", err)
	}
	// Remove trailing newline added by Encoder
	dataJSON := bytes.TrimRight(buffer.Bytes(), "\n")

	// 2. Build system prompt
	systemPrompt := buildSystemPrompt(req.AcceptLanguage, string(dataJSON))

	// 3. Build contents array (system + history + current message)
	contents := buildContents(systemPrompt, req.History, req.Message)

	// 4. Call Gemini API
	geminiResp, err := callGeminiAPI(apiKey, contents)
	if err != nil {
		return "", err
	}

	// 5. Convert any HTML links to markdown (in case Gemini ignores instructions)
	response := convertHTMLLinksToMarkdown(geminiResp)

	// 6. Transform IRIs to markdown links with /resource/ prefix
	response = transformIRIsToLinks(response)

	return response, nil
}

// buildSystemPrompt creates the system prompt with instructions and data
func buildSystemPrompt(language, dataJSON string) string {
	return fmt.Sprintf(`You are a helpful assistant answering questions about RDF resources.

STRICT RULES:
1. Answer ONLY based on the provided SPARQL query results below
2. Do NOT invent or assume information not present in the data
3. If you don't have enough information to answer, clearly state: "I don't have enough information to answer that question."
4. When referring to other resources (URIs), format them ONLY as markdown links: [Label](URI) - NEVER use HTML tags
5. IMPORTANT: Use markdown syntax for links, NOT HTML. Write [text](url) not <a href="url">text</a>
6. Respond in the following language: %s
7. Be concise but informative - prefer 2-3 sentence answers unless more detail is requested
8. Use the "Lol" field value for display when available (it contains the human-readable label)

RESOURCE DATA (SPARQL Query Results):
%s`, language, dataJSON)
}

// buildContents creates the Gemini API contents array from system prompt, history, and current message
func buildContents(systemPrompt string, history []Message, currentMsg string) []geminiContent {
	var contents []geminiContent

	// First message: system prompt as user message
	contents = append(contents, geminiContent{
		Role: "user",
		Parts: []geminiPart{
			{Text: systemPrompt},
		},
	})

	// Add acknowledgment from model
	contents = append(contents, geminiContent{
		Role: "model",
		Parts: []geminiPart{
			{Text: "I understand. I will answer questions based only on the provided SPARQL data and indicate clearly when I don't have enough information."},
		},
	})

	// Add conversation history
	for _, msg := range history {
		role := "user"
		if msg.Role == "assistant" {
			role = "model"
		}
		contents = append(contents, geminiContent{
			Role: role,
			Parts: []geminiPart{
				{Text: msg.Content},
			},
		})
	}

	// Add current message
	contents = append(contents, geminiContent{
		Role: "user",
		Parts: []geminiPart{
			{Text: currentMsg},
		},
	})

	return contents
}

// callGeminiAPI makes the HTTP request to Gemini API
func callGeminiAPI(apiKey string, contents []geminiContent) (string, error) {
	// Build request
	reqBody := geminiRequest{
		Contents: contents,
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("failed to marshal request: %w", err)
	}

	// Create HTTP request
	url := fmt.Sprintf("%s?key=%s", geminiEndpoint, apiKey)
	httpReq, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")

	// Make request with timeout
	client := &http.Client{
		Timeout: geminiTimeout,
	}
	resp, err := client.Do(httpReq)
	if err != nil {
		return "", fmt.Errorf("API request failed: %w", err)
	}
	defer resp.Body.Close()

	// Read response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response: %w", err)
	}

	// Check HTTP status
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("API returned status %d: %s", resp.StatusCode, string(body))
	}

	// Parse response
	var geminiResp geminiResponse
	if err := json.Unmarshal(body, &geminiResp); err != nil {
		return "", fmt.Errorf("failed to parse response: %w", err)
	}

	// Extract text from first candidate
	if len(geminiResp.Candidates) == 0 || len(geminiResp.Candidates[0].Content.Parts) == 0 {
		return "", fmt.Errorf("no response from API")
	}

	return geminiResp.Candidates[0].Content.Parts[0].Text, nil
}

// convertHTMLLinksToMarkdown converts HTML anchor tags to markdown format
// Converts <a href="url">text</a> to [text](url)
func convertHTMLLinksToMarkdown(text string) string {
	// Match <a href="url">text</a> pattern
	htmlLinkPattern := regexp.MustCompile(`<a\s+href="([^"]+)">([^<]+)</a>`)
	result := htmlLinkPattern.ReplaceAllString(text, "[$2]($1)")
	return result
}

// transformIRIsToLinks converts bare URIs in the text to markdown links
// Format: https://example.com/resource -> [label](/resource/https://example.com/resource)
// Also transforms existing markdown links to use /resource/ prefix
func transformIRIsToLinks(text string) string {
	// First, handle URIs that are already in markdown format: [text](https://...)
	// Transform them to use /resource/ prefix if they don't already have it
	markdownPattern := regexp.MustCompile(`\[([^\]]+)\]\((https?://[^)]+)\)`)
	result := markdownPattern.ReplaceAllStringFunc(text, func(match string) string {
		submatches := markdownPattern.FindStringSubmatch(match)
		if len(submatches) == 3 {
			label := submatches[1]
			uri := submatches[2]
			// Only add /resource/ if not already present
			if !strings.HasPrefix(uri, "/resource/") {
				return fmt.Sprintf("[%s](/resource/%s)", label, uri)
			}
		}
		return match
	})

	// Then, handle bare URIs not already in markdown link format
	// Only match URIs that have whitespace or start of string before them, and whitespace or punctuation after
	// This ensures we don't match URIs inside markdown links: [text](URI)
	bareUriPattern := regexp.MustCompile(`(^|[\s])(https?://[^\s)\]]+)([\s.,;!?]|$)`)
	result = bareUriPattern.ReplaceAllStringFunc(result, func(match string) string {
		submatches := bareUriPattern.FindStringSubmatch(match)
		if len(submatches) == 4 {
			prefix := submatches[1]
			uri := submatches[2]
			suffix := submatches[3]

			// Extract label from URI (last segment)
			label := extractLabel(uri)

			// Return markdown link with /resource/ prefix
			return fmt.Sprintf("%s[%s](/resource/%s)%s", prefix, label, uri, suffix)
		}
		return match
	})

	return result
}

// extractLabel extracts a human-readable label from a URI
// Takes the last segment after the last / or #
func extractLabel(uri string) string {
	uri = strings.TrimSpace(uri)

	// Try to extract after last / or #
	lastSlash := strings.LastIndex(uri, "/")
	lastHash := strings.LastIndex(uri, "#")

	pos := lastSlash
	if lastHash > pos {
		pos = lastHash
	}

	if pos >= 0 && pos < len(uri)-1 {
		label := uri[pos+1:]
		// Decode URL-encoded characters
		label = strings.ReplaceAll(label, "%3A", ":")
		label = strings.ReplaceAll(label, "%20", " ")
		return label
	}

	return uri
}

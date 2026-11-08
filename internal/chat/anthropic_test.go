package chat

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/pdacton/visoto/internal/config"
)

// fakeAnthropic answers /v1/messages: a tool call first, then text.
type fakeAnthropic struct {
	*httptest.Server
	mu     sync.Mutex
	bodies []map[string]any
	keys   []string
	betas  []string
	status int
}

func newFakeAnthropic(t *testing.T) *fakeAnthropic {
	f := &fakeAnthropic{}
	f.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		var body map[string]any
		_ = json.Unmarshal(raw, &body)
		f.mu.Lock()
		f.bodies = append(f.bodies, body)
		f.keys = append(f.keys, r.Header.Get("Authorization"))
		f.betas = append(f.betas, r.Header.Get("Anthropic-Beta"))
		n := len(f.bodies)
		status := f.status
		f.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		if status != 0 {
			w.WriteHeader(status)
			_, _ = io.WriteString(w, `{"type":"error","error":{"type":"authentication_error","message":"invalid x-api-key"}}`)
			return
		}
		if n == 1 {
			_, _ = io.WriteString(w, `{"id":"m1","type":"message","role":"assistant","model":"claude-opus-5-5",
				"content":[{"type":"tool_use","id":"tu1","name":"list_prefixes","input":{}}],
				"stop_reason":"tool_use","usage":{"input_tokens":1,"output_tokens":1}}`)
			return
		}
		_, _ = io.WriteString(w, `{"id":"m2","type":"message","role":"assistant","model":"claude-opus-5-5",
			"content":[{"type":"text","text":"See <a href=\"https://ex.test/a\">A</a>."}],
			"stop_reason":"end_turn","usage":{"input_tokens":1,"output_tokens":1}}`)
	}))
	t.Cleanup(f.Close)
	anthropicBaseURL = f.URL
	t.Cleanup(func() { anthropicBaseURL = "" })
	return f
}

func TestCallAnthropicRunsTheToolLoop(t *testing.T) {
	f := newFakeAnthropic(t)
	t.Setenv("ANTHROPIC_API_KEY", "sk-server-must-not-be-used")
	req := ChatRequest{Message: "What prefixes?", History: []Message{{Role: "user", Content: "hi"}, {Role: "assistant", Content: "hello"}}}
	// No MCP server listens here: the tool result is an error the model sees.
	out, err := CallAnthropic(context.Background(), "sk-test", "http://127.0.0.1:1/mcp", req)
	if err != nil {
		t.Fatal(err)
	}
	if out != "See [A](/resource?iri=https%3A%2F%2Fex.test%2Fa)." {
		t.Errorf("answer = %q", out)
	}
	if len(f.bodies) != 2 {
		t.Fatalf("%d requests, want 2", len(f.bodies))
	}
	first := f.bodies[0]
	if first["model"] != "claude-opus-5-5" || first["fallbacks"] != "default" || f.keys[0] != "Bearer sk-test" ||
		!strings.Contains(f.betas[0], "server-side-fallback-2026-07-01") {
		t.Errorf("first request: model=%v fallbacks=%v key=%q beta=%q", first["model"], first["fallbacks"], f.keys[0], f.betas[0])
	}
	if eff := first["output_config"].(map[string]any)["effort"]; eff != "medium" {
		t.Errorf("effort = %v", eff)
	}
	if tools := first["tools"].([]any); len(tools) == 0 {
		t.Error("no tools offered")
	}
	// The second request carries the assistant's tool_use and our tool_result.
	msgs := f.bodies[1]["messages"].([]any)
	last := msgs[len(msgs)-1].(map[string]any)
	block := last["content"].([]any)[0].(map[string]any)
	if last["role"] != "user" || block["type"] != "tool_result" || block["tool_use_id"] != "tu1" || block["is_error"] != true {
		t.Errorf("tool result turn = %v", last)
	}
	if prev := msgs[len(msgs)-2].(map[string]any); prev["role"] != "assistant" {
		t.Errorf("assistant turn not kept: %v", prev)
	}
}

func TestCallAnthropicRejectedKey(t *testing.T) {
	f := newFakeAnthropic(t)
	f.status = http.StatusUnauthorized
	_, err := CallAnthropic(context.Background(), "bad", "http://127.0.0.1:1/mcp", ChatRequest{Message: "x"})
	if !errors.Is(err, ErrInvalidKey) {
		t.Errorf("err = %v, want ErrInvalidKey", err)
	}
}

// The caller's own key is used even when the instance has no Gemini key.
func TestHandlerUsesTheCallersKey(t *testing.T) {
	newFakeAnthropic(t)
	gin.SetMode(gin.TestMode)
	creds := func() CredentialsFunc {
		return func(ctx context.Context) (Credentials, bool) {
			return Credentials{Provider: "anthropic", APIKey: "sk-own"}, true
		}
	}
	r := gin.New()
	r.POST("/chat", Handler(&config.ApplicationConfig{}, "", "http://127.0.0.1:1/mcp", creds))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest("POST", "/chat", strings.NewReader(`{"message":"q"}`)))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "[A](/resource?iri=") {
		t.Errorf("own key: %d %s", rec.Code, rec.Body.String())
	}

	r = gin.New()
	r.POST("/chat", Handler(&config.ApplicationConfig{}, "", "http://127.0.0.1:1/mcp", nil))
	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest("POST", "/chat", strings.NewReader(`{"message":"q"}`)))
	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("no key at all: %d", rec.Code)
	}
}

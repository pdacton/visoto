package chat

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/pdacton/visoto/internal/config"
	"github.com/pdacton/visoto/internal/logger"
)

// Credentials is an AI provider key the caller brings (commercial edition,
// UA-50). Provider is "anthropic"; other providers' keys are ignored here.
type Credentials struct {
	Provider string
	APIKey   string
}

// CredentialsFunc returns the caller's own provider key, if any. ctx is the
// HTTP request's context.
type CredentialsFunc func(ctx context.Context) (Credentials, bool)

// Handler returns a Gin handler function for chat requests.
// apiKey is the instance's Gemini API key; mcpURL is the base URL of the Visoto
// MCP server (e.g. http://localhost:8060/mcp). creds, when it returns an
// Anthropic key for the caller, makes the request use Claude with that key
// instead. cfg resolves the request's endpoint slug.
func Handler(cfg *config.ApplicationConfig, apiKey, mcpURL string, creds func() CredentialsFunc) gin.HandlerFunc {
	return func(c *gin.Context) {
		log := logger.Get()

		var own Credentials
		if creds != nil {
			if f := creds(); f != nil {
				if k, ok := f(c.Request.Context()); ok && k.Provider == "anthropic" && k.APIKey != "" {
					own = k
				}
			}
		}

		// Validate API key is configured
		if apiKey == "" && own.APIKey == "" {
			log.Error("gemini API key not configured")
			c.JSON(http.StatusServiceUnavailable, ChatResponse{
				Error: "Chat service is not configured",
			})
			return
		}

		// Parse JSON request body
		var req ChatRequest
		if err := c.BindJSON(&req); err != nil {
			log.Error("invalid chat request", slog.String("error", err.Error()))
			c.JSON(http.StatusBadRequest, ChatResponse{
				Error: "Invalid request format",
			})
			return
		}

		ep := cfg.GetEndpointBySlug(req.Endpoint)
		if ep == nil {
			ep = cfg.DefaultEndpoint()
		}
		if ep != nil {
			req.endpointName = ep.Name
		}

		log.Debug("processing chat request",
			slog.String("resourceIRI", req.ResourceIRI),
			slog.String("message", req.Message),
			slog.Int("historyLength", len(req.History)))

		// Call the model (with MCP tool support): the caller's own Claude key
		// if they brought one, else the instance's Gemini key.
		var response string
		var err error
		if own.APIKey != "" {
			response, err = CallAnthropic(c.Request.Context(), own.APIKey, mcpURL, req)
		} else {
			response, err = CallGemini(apiKey, mcpURL, req)
		}
		if errors.Is(err, ErrInvalidKey) {
			c.JSON(http.StatusBadRequest, ChatResponse{
				Error: "Your Anthropic API key was rejected. Check it on your account page.",
			})
			return
		}
		if err != nil {
			log.Error("chat API call failed", slog.String("error", err.Error()), slog.Bool("ownKey", own.APIKey != ""))
			if strings.HasPrefix(err.Error(), "rate_limit:") {
				delay := strings.TrimPrefix(err.Error(), "rate_limit: ")
				c.JSON(http.StatusTooManyRequests, ChatResponse{
					Error: "Rate limit reached. Please wait " + delay + " and try again.",
				})
			} else {
				c.JSON(http.StatusInternalServerError, ChatResponse{
					Error: "Failed to generate response. Please try again.",
				})
			}
			return
		}

		log.Debug("chat response generated", slog.Int("responseLength", len(response)))

		c.JSON(http.StatusOK, ChatResponse{
			Response: response,
		})
	}
}

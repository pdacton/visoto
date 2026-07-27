package chat

import (
	"log/slog"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"hutzli.org/visoto/internal/logger"
)

// Handler returns a Gin handler function for chat requests.
// apiKey is the Gemini API key; mcpURL is the base URL of the Visoto MCP server (e.g. http://localhost:8060/mcp).
func Handler(apiKey, mcpURL string) gin.HandlerFunc {
	return func(c *gin.Context) {
		log := logger.Get()

		// Validate API key is configured
		if apiKey == "" {
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

		log.Debug("processing chat request",
			slog.String("resourceIRI", req.ResourceIRI),
			slog.String("message", req.Message),
			slog.Int("historyLength", len(req.History)))

		// Call Gemini API (with MCP tool support)
		response, err := CallGemini(apiKey, mcpURL, req)
		if err != nil {
			log.Error("gemini API call failed", slog.String("error", err.Error()))
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

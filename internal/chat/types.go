package chat

// ChatRequest represents the incoming chat request from the frontend
type ChatRequest struct {
	ResourceIRI    string                 `json:"resourceIRI"`
	Message        string                 `json:"message"`
	History        []Message              `json:"history"`
	Data           map[string]interface{} `json:"data"`
	AcceptLanguage string                 `json:"acceptLanguage"`
}

// Message represents a single chat message in the conversation history
type Message struct {
	Role    string `json:"role"`    // "user" or "assistant"
	Content string `json:"content"` // message text
}

// ChatResponse represents the API response sent back to the frontend
type ChatResponse struct {
	Response string `json:"response"`          // LLM-generated response
	Error    string `json:"error,omitempty"`   // error message if any
}

package chat

// ChatRequest represents the incoming chat request from the frontend
type ChatRequest struct {
	ResourceIRI    string                 `json:"resourceIRI"`
	Message        string                 `json:"message"`
	History        []Message              `json:"history"`
	Data           map[string]interface{} `json:"data"`
	AcceptLanguage string                 `json:"acceptLanguage"`
	ActiveEndpoint ActiveEndpointInfo     `json:"activeEndpoint"`
}

// ActiveEndpointInfo carries the name and URL of the currently selected SPARQL endpoint.
type ActiveEndpointInfo struct {
	Name string `json:"name"`
	URL  string `json:"url"`
}

// Message represents a single chat message in the conversation history
type Message struct {
	Role    string `json:"role"`    // "user" or "assistant"
	Content string `json:"content"` // message text
}

// ChatResponse represents the API response sent back to the frontend
type ChatResponse struct {
	Response string `json:"response"`        // LLM-generated response
	Error    string `json:"error,omitempty"` // error message if any
}

// --- Gemini API types ---

type geminiRequest struct {
	Contents   []geminiContent   `json:"contents"`
	Tools      []geminiTool      `json:"tools,omitempty"`
	ToolConfig *geminiToolConfig `json:"toolConfig,omitempty"`
}

// geminiToolConfig controls how the model uses tools.
// Mode "AUTO" lets the model decide; "ANY" forces a tool call; "NONE" disables tools.
type geminiToolConfig struct {
	FunctionCallingConfig geminiFunctionCallingConfig `json:"functionCallingConfig"`
}

type geminiFunctionCallingConfig struct {
	Mode string `json:"mode"` // "AUTO", "ANY", or "NONE"
}

type geminiContent struct {
	Role  string       `json:"role"` // "user" or "model"
	Parts []geminiPart `json:"parts"`
}

type geminiPart struct {
	Text             string                  `json:"text,omitempty"`
	Thought          bool                    `json:"thought,omitempty"`
	ThoughtSignature string                  `json:"thoughtSignature,omitempty"`
	FunctionCall     *geminiFunctionCall     `json:"functionCall,omitempty"`
	FunctionResponse *geminiFunctionResponse `json:"functionResponse,omitempty"`
}

type geminiFunctionCall struct {
	Name string         `json:"name"`
	Args map[string]any `json:"args"`
}

type geminiFunctionResponse struct {
	Name     string         `json:"name"`
	Response map[string]any `json:"response"`
}

type geminiTool struct {
	FunctionDeclarations []geminiFunctionDecl `json:"function_declarations"`
}

type geminiFunctionDecl struct {
	Name        string             `json:"name"`
	Description string             `json:"description"`
	Parameters  geminiSchemaObject `json:"parameters"`
}

type geminiSchemaObject struct {
	Type       string                      `json:"type"`
	Properties map[string]geminiSchemaProp `json:"properties,omitempty"`
	Required   []string                    `json:"required,omitempty"`
}

type geminiSchemaProp struct {
	Type        string `json:"type"`
	Description string `json:"description,omitempty"`
}

type geminiResponse struct {
	Candidates []geminiCandidate `json:"candidates"`
}

type geminiCandidate struct {
	Content geminiContent `json:"content"`
}

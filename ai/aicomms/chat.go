package aicomms

import (
	"encoding/json"
	"time"
)

type ChatRequest struct {
	Model     string            `json:"model"`
	Messages  []ChatMessage     `json:"messages"`
	Tools     []json.RawMessage `json:"tools,omitempty"`
	Format    string            `json:"format,omitempty"`
	Options   map[string]any    `json:"options,omitempty"`
	Stream    bool              `json:"stream"`
	KeepAlive *time.Duration    `json:"keep_alive,omitempty"`
}

type ChatResponse struct {
	Model              string      `json:"model"`
	CreatedAt          time.Time   `json:"created_at"`
	Message            ChatMessage `json:"message"`
	Done               bool        `json:"done"`
	Context            []int       `json:"context"`
	TotalDuration      int64       `json:"total_duration"`
	LoadDuration       int64       `json:"load_duration"`
	PromptEvalCount    int         `json:"prompt_eval_count"`
	PromptEvalDuration int64       `json:"prompt_eval_duration"`
	EvalCount          int         `json:"eval_count"`
	EvalDuration       int64       `json:"eval_duration"`
}

type ChatMessage struct {
	Role      string            `json:"role"`
	Content   string            `json:"content"`
	Images    []string          `json:"images,omitempty"`
	ToolCalls []json.RawMessage `json:"tool_calls,omitempty"`
}

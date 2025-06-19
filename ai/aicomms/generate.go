package aicomms

import "time"

type GenerateRequest struct {
	// Standard params
	Model  string `json:"model"`
	Prompt string `json:"prompt,omitempty"`
	Suffix string `json:"suffix,omitempty"`
	Images string `json:"images,omitempty"`
	// Advanced params
	Format    string         `json:"format,omitempty"`
	Options   map[string]any `json:"options,omitempty"`
	System    string         `json:"system,omitempty"`
	Template  string         `json:"template,omitempty"`
	Stream    bool           `json:"stream"`
	Raw       bool           `json:"raw"`
	KeepAlive *time.Duration `json:"keep_alive,omitempty"`
}

type GenerateResponse struct {
	GenerateStream
	Context            []int `json:"context"`
	TotalDuration      int64 `json:"total_duration"`
	LoadDuration       int64 `json:"load_duration"`
	PromptEvalCount    int   `json:"prompt_eval_count"`
	PromptEvalDuration int64 `json:"prompt_eval_duration"`
	EvalCount          int   `json:"eval_count"`
	EvalDuration       int64 `json:"eval_duration"`
}

type GenerateStream struct {
	Model     string    `json:"model"`
	CreatedAt time.Time `json:"created_at"`
	Response  string    `json:"response"`
	Done      bool      `json:"done"`
}

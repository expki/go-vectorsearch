package aicomms

import (
	"encoding/json"
	"time"

	"github.com/expki/go-vectorsearch/compute"
	"github.com/expki/go-vectorsearch/config"
)

type EmbedRequest struct {
	// Standard params
	Model string                       `json:"model"`
	Input config.SingleOrSlice[string] `json:"input"`
	// Advanced params
	Truncate  *bool          `json:"truncate,omitempty"`
	Options   map[string]any `json:"options,omitempty"`
	KeepAlive *time.Duration `json:"keep_alive,omitempty"`
}

type EmbedResponse struct {
	Model           string     `json:"model"`
	Embeddings      Embeddings `json:"embeddings"`
	Done            bool       `json:"done"`
	TotalDuration   int64      `json:"total_duration"`
	LoadDuration    int64      `json:"load_duration"`
	PromptEvalCount int        `json:"prompt_eval_count"`
}

type Embeddings []Embedding

func (e Embeddings) Value() [][]uint8 {
	value := make([][]uint8, len(e))
	for i, v := range e {
		value[i] = v
	}
	return value
}

type Embedding []uint8

func (e *Embedding) UnmarshalJSON(data []byte) error {
	var vector []float32
	err := json.Unmarshal(data, &vector)
	if err != nil {
		return err
	}
	*e = compute.QuantizeVectorFloat32(vector)
	return nil
}

func (e Embedding) Dims() int {
	return len(e) - 8
}

func (e Embedding) Value() []uint8 {
	return e
}

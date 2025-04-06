package config

import (
	_ "github.com/expki/go-vectorsearch/env"
)

type AI struct {
	Embed    Ollama `json:"embed"`
	Generate Ollama `json:"generate"`
	Chat     Ollama `json:"chat"`
}

type Ollama struct {
	Url    SingleOrSlice[string] `json:"url"`
	Token  string                `json:"token"`
	Model  string                `json:"model"`
	NumCtx int                   `json:"num_ctx"`
}

func (c Ollama) GetNumCtx() int {
	if c.NumCtx <= 0 {
		return 512
	}
	return c.NumCtx
}

package config

import (
	_ "github.com/expki/go-vectorsearch/env"
)

type Ollama struct {
	Url      SingleOrSlice[string] `json:"url"`
	Token    string                `json:"token"`
	Embed    string                `json:"embed"`
	Generate string                `json:"generate"`
	Chat     string                `json:"chat"`
}

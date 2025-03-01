package config

import (
	_ "github.com/expki/govecdb/env"
)

type Ollama struct {
	Url      string `json:"url"`
	Token    string `json:"token"`
	Embed    string `json:"embed"`
	Generate string `json:"generate"`
	Chat     string `json:"chat"`
}

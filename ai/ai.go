package ai

import (
	"errors"

	"github.com/expki/go-vectorsearch/ai/ollama"
	"github.com/expki/go-vectorsearch/ai/openai"
	"github.com/expki/go-vectorsearch/config"
	_ "github.com/expki/go-vectorsearch/env"
)

type ai struct {
	ollama *ollama.Ollama
	openai *openai.OpenAI
}

func New(ollamaCfg, openaiCfg config.AI) (AI, error) {
	ollama, err := ollama.New(ollamaCfg)
	if err != nil {
		return nil, errors.Join(errors.New("Ollama client"), err)
	}
	openai, err := openai.New(openaiCfg)
	if err != nil {
		return nil, errors.Join(errors.New("OpenAI client"), err)
	}
	return &ai{
		ollama: ollama,
		openai: openai,
	}, nil
}

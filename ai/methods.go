package ai

import (
	"context"
	"errors"
	"io"
	"math"

	"github.com/expki/go-vectorsearch/ai/aicomms"
	_ "github.com/expki/go-vectorsearch/env"
)

// Embed generates vector embeddings from the input text provided in the request.
func (ai *ai) Embed(ctx context.Context, request aicomms.EmbedRequest) (response aicomms.EmbedResponse, err error) {
	if ai.ollama.CanEmbed() {
		return ai.ollama.Embed(ctx, request)
	}
	if ai.openai.CanEmbed() {
		return ai.openai.Embed(ctx, request)
	}
	return response, errors.New("no embed provider configured")
}

// Generate creates new content based on the prompt in a single response.
func (ai *ai) Generate(ctx context.Context, request aicomms.GenerateRequest) (response aicomms.GenerateResponse, err error) {
	if ai.ollama.CanGenerate() {
		return ai.ollama.Generate(ctx, request)
	}
	if ai.openai.CanGenerate() {
		return ai.openai.Generate(ctx, request)
	}
	return response, errors.New("no embed provider configured")
}

// Generate creates new content based on the prompt as an byte stream.
func (ai *ai) GenerateStream(ctx context.Context, request aicomms.GenerateRequest) (stream io.Reader) {
	if ai.ollama.CanGenerate() {
		return ai.ollama.GenerateStream(ctx, request)
	}
	if ai.openai.CanGenerate() {
		return ai.openai.GenerateStream(ctx, request)
	}
	return nil
}

// Chat facilitates a conversation between the AI and a user with documentation as context in a single response.
func (ai *ai) Chat(ctx context.Context, request aicomms.ChatRequest) (response aicomms.ChatResponse, err error) {
	if ai.ollama.CanChat() {
		return ai.ollama.Chat(ctx, request)
	}
	if ai.openai.CanChat() {
		return ai.openai.Chat(ctx, request)
	}
	return response, errors.New("no embed provider configured")
}

// Chat facilitates a conversation between the AI and a user with documentation as context as a byte stream.
func (ai *ai) ChatStream(ctx context.Context, request aicomms.ChatRequest) (stream io.ReadCloser) {
	if ai.ollama.CanChat() {
		return ai.ollama.ChatStream(ctx, request)
	}
	if ai.openai.CanChat() {
		return ai.openai.ChatStream(ctx, request)
	}
	return nil
}

// EmbedCtxNum returns the supported input context size.
func (ai *ai) EmbedCtxNum() (ctxnum int) {
	if ai.ollama.CanEmbed() {
		return ai.ollama.Cfg.Embed.GetNumCtx()
	}
	if ai.openai.CanEmbed() {
		return ai.openai.Cfg.Embed.GetNumCtx()
	}
	return -math.MaxInt
}

// GenerateCtxNum returns the supported input context size.
func (ai *ai) GenerateCtxNum() (ctxnum int) {
	if ai.ollama.CanGenerate() {
		return ai.ollama.Cfg.Generate.GetNumCtx()
	}
	if ai.openai.CanGenerate() {
		return ai.openai.Cfg.Generate.GetNumCtx()
	}
	return -math.MaxInt
}

// ChatCtxNum returns the supported input context size.
func (ai *ai) ChatCtxNum() (ctxnum int) {
	if ai.ollama.CanChat() {
		return ai.ollama.Cfg.Chat.GetNumCtx()
	}
	if ai.openai.CanChat() {
		return ai.openai.Cfg.Chat.GetNumCtx()
	}
	return -math.MaxInt
}

// EmbedModel returns the target model.
func (ai *ai) EmbedModel() (model string) {
	if ai.ollama.CanEmbed() {
		return ai.ollama.Cfg.Embed.Model
	}
	if ai.openai.CanEmbed() {
		return ai.openai.Cfg.Embed.Model
	}
	return
}

// GenerateModel returns the target model.
func (ai *ai) GenerateModel() (model string) {
	if ai.ollama.CanGenerate() {
		return ai.ollama.Cfg.Generate.Model
	}
	if ai.openai.CanGenerate() {
		return ai.openai.Cfg.Generate.Model
	}
	return
}

// ChatModel returns the target model.
func (ai *ai) ChatModel() (model string) {
	if ai.ollama.CanChat() {
		return ai.ollama.Cfg.Chat.Model
	}
	if ai.openai.CanChat() {
		return ai.openai.Cfg.Chat.Model
	}
	return
}

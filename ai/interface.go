package ai

import (
	"context"
	"io"

	_ "github.com/expki/go-vectorsearch/env"
)

// AI represents an interface for interacting with various AI services.
type AI interface {
	// Embed generates vector embeddings from the input text provided in the request.
	Embed(ctx context.Context, request EmbedRequest) (response EmbedResponse, err error)

	// Generate creates new content based on the prompt in a single response.
	Generate(ctx context.Context, request GenerateRequest) (response GenerateResponse, err error)

	// Generate creates new content based on the prompt as an byte stream.
	GenerateStream(ctx context.Context, request GenerateRequest) (stream io.Reader)

	// Chat facilitates a conversation between the AI and a user with documentation as context in a single response.
	Chat(ctx context.Context, request ChatRequest) (response ChatResponse, err error)

	// Chat facilitates a conversation between the AI and a user with documentation as context as a byte stream.
	ChatStream(ctx context.Context, request ChatRequest) (stream io.Reader)
}

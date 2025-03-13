package noop

import (
	"bytes"
	"context"
	crand "crypto/rand"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"io"
	"math/rand"
	"time"

	"github.com/expki/go-vectorsearch/ai"
	_ "github.com/expki/go-vectorsearch/env"
)

const (
	embeddingVectorSize = 512
	generateMaxLength   = 512
)

type noai struct {
	random *rand.Rand
}

func NoAI() ai.AI {
	var seed int64
	raw := make([]byte, 8)
	_, err := crand.Read(raw)
	if err == nil {
		raw[7] &= 0x7F // Ensure it is a positive number.
		seed = int64(binary.LittleEndian.Uint64(raw))
	} else {
		seed = time.Now().Unix()
	}
	return &noai{
		random: rand.New(rand.NewSource(seed)),
	}
}

func (n *noai) Embed(_ context.Context, request ai.EmbedRequest) (response ai.EmbedResponse, err error) {
	if len(request.Input) == 0 {
		return response, errors.New("input is empty")
	}
	response.Embeddings = make(ai.Embeddings, len(request.Input))
	for idx := range len(request.Input) {
		raw := make([]byte, embeddingVectorSize)
		n.random.Read(raw)
		row := make([]ai.EmbeddingValue, embeddingVectorSize)
		for n, val := range raw {
			row[n] = ai.EmbeddingValue(val)
		}
		response.Embeddings[idx] = row
	}
	return
}

func (n *noai) Generate(_ context.Context, request ai.GenerateRequest) (response ai.GenerateResponse, err error) {
	raw := make([]byte, n.random.Intn(generateMaxLength))
	n.random.Read(raw)
	response.Response = hex.EncodeToString(raw)
	return
}

func (n *noai) GenerateStream(_ context.Context, request ai.GenerateRequest) (stream io.Reader) {
	raw := make([]byte, n.random.Intn(generateMaxLength))
	n.random.Read(raw)
	return bytes.NewBuffer([]byte(hex.EncodeToString(raw)))
}

func (n *noai) Chat(_ context.Context, request ai.ChatRequest) (response ai.ChatResponse, err error) {
	raw := make([]byte, n.random.Intn(generateMaxLength))
	n.random.Read(raw)
	response.Message.Content = hex.EncodeToString(raw)
	return
}

func (n *noai) ChatStream(_ context.Context, request ai.ChatRequest) (stream io.Reader) {
	raw := make([]byte, n.random.Intn(generateMaxLength))
	n.random.Read(raw)
	return bytes.NewBuffer([]byte(hex.EncodeToString(raw)))
}

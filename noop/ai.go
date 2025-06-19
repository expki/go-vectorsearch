package noop

import (
	"bytes"
	"context"
	crand "crypto/rand"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"io"
	"math"
	"math/rand"
	"time"

	"github.com/expki/go-vectorsearch/ai"
	"github.com/expki/go-vectorsearch/ai/aicomms"
	"github.com/expki/go-vectorsearch/config"
	_ "github.com/expki/go-vectorsearch/env"
)

const (
	embeddingVectorSize = 512
	generateMaxLength   = 512
)

type noai struct {
	random *rand.Rand
}

// NewOllama implementes a fake ai.NewOllama
func NewOllama(_ config.Provider) (ai ai.AI, _ error) {
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
	}, nil
}

// Embed implementes a fake ai.Embed
func (n *noai) Embed(_ context.Context, request aicomms.EmbedRequest) (response aicomms.EmbedResponse, err error) {
	if len(request.Input) == 0 {
		return response, errors.New("input is empty")
	}
	response.Embeddings = make(aicomms.Embeddings, len(request.Input))
	for idx := range len(request.Input) {
		row := make([]byte, 8+embeddingVectorSize)
		binary.LittleEndian.PutUint32(row, math.Float32bits(-1))
		binary.LittleEndian.PutUint32(row[4:], math.Float32bits(1))

		raw := make([]byte, embeddingVectorSize)
		n.random.Read(raw)

		copy(row[8:], raw)
		response.Embeddings[idx] = row
	}
	return
}

// Generate implementes a fake ai.Generate
func (n *noai) Generate(_ context.Context, request aicomms.GenerateRequest) (response aicomms.GenerateResponse, err error) {
	raw := make([]byte, n.random.Intn(generateMaxLength))
	n.random.Read(raw)
	response.Response = hex.EncodeToString(raw)
	return
}

// GenerateStream implementes a fake ai.GenerateStream
func (n *noai) GenerateStream(_ context.Context, request aicomms.GenerateRequest) (stream io.Reader) {
	raw := make([]byte, n.random.Intn(generateMaxLength))
	n.random.Read(raw)
	return bytes.NewBuffer([]byte(hex.EncodeToString(raw)))
}

// Chat implementes a fake ai.Chat
func (n *noai) Chat(_ context.Context, request aicomms.ChatRequest) (response aicomms.ChatResponse, err error) {
	raw := make([]byte, n.random.Intn(generateMaxLength))
	n.random.Read(raw)
	response.Message.Content = hex.EncodeToString(raw)
	return
}

// ChatStream implementes a fake ai.ChatStream
func (n *noai) ChatStream(_ context.Context, request aicomms.ChatRequest) (stream io.ReadCloser) {
	raw := make([]byte, n.random.Intn(generateMaxLength))
	n.random.Read(raw)
	return io.NopCloser(bytes.NewBuffer([]byte(hex.EncodeToString(raw))))
}

// EmbedCtxNum returns the supported input context size.
func (*noai) EmbedCtxNum() (ctxnum int) {
	return -math.MaxInt
}

// GenerateCtxNum returns the supported input context size.
func (*noai) GenerateCtxNum() (ctxnum int) {
	return -math.MaxInt
}

// ChatCtxNum returns the supported input context size.
func (*noai) ChatCtxNum() (ctxnum int) {
	return -math.MaxInt
}

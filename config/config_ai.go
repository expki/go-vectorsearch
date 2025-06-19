package config

import (
	"math"

	_ "github.com/expki/go-vectorsearch/env"
)

type AI struct {
	Embed    *Provider `json:"embed"`
	Generate *Provider `json:"generate"`
	Chat     *Provider `json:"chat"`
}

type Provider struct {
	Model              string                `json:"model"`
	ApiBase            SingleOrSlice[string] `json:"api_base"`
	ApiKey             string                `json:"api_key"`
	NumCtx             int                   `json:"num_ctx"`
	RequestCompression bool                  `json:"request_compression"`
}

func (c Provider) GetNumCtx() int {
	if c.NumCtx <= 0 {
		return -math.MaxInt
	}
	return c.NumCtx
}

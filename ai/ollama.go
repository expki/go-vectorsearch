package ai

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"

	"github.com/expki/go-vectorsearch/config"
	_ "github.com/expki/go-vectorsearch/env"
	"golang.org/x/net/http2"
)

type Ollama struct {
	uri    url.URL
	client *http.Client
	token  string
}

func NewOllama(cfg config.Ollama) (ai *Ollama, err error) {
	ai = new(Ollama)

	// Parse Ollama URI
	uriPonter, err := url.Parse(cfg.Url)
	if err != nil {
		return nil, fmt.Errorf("unable to parse ollama url '%s': %v", cfg.Url, err)
	} else if uriPonter == nil {
		return nil, errors.New("parsed ollama url is nil")
	}
	ai.uri = *uriPonter
	ai.token = cfg.Token

	// Create http client
	transport := &http.Transport{}
	http2.ConfigureTransport(transport)
	ai.client = &http.Client{
		Transport: transport,
	}

	return ai, nil
}

package ai

import (
	"errors"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"sync/atomic"

	"github.com/expki/go-vectorsearch/config"
	_ "github.com/expki/go-vectorsearch/env"
	"golang.org/x/net/http2"
)

type Ollama struct {
	uri    []*ollamaUrl
	client *http.Client
	token  string
}

func NewOllama(cfg config.Ollama) (ai *Ollama, err error) {
	ai = new(Ollama)

	// Parse Ollama URI
	for _, cfgUrl := range cfg.Url {
		uriPonter, err := url.Parse(cfgUrl)
		if err != nil {
			return nil, fmt.Errorf("unable to parse ollama url '%s': %v", cfgUrl, err)
		} else if uriPonter == nil {
			return nil, errors.New("parsed ollama url is nil")
		}
		ai.uri = append(ai.uri, &ollamaUrl{
			uri: *uriPonter,
		})
	}
	ai.token = cfg.Token

	// Create http client
	transport := &http.Transport{}
	http2.ConfigureTransport(transport)
	ai.client = &http.Client{
		Transport: transport,
	}

	return ai, nil
}

func (o *Ollama) Url() *ollamaUrl {
	var best *ollamaUrl
	var bestConns int64 = math.MaxInt64
	for _, uri := range o.uri {
		if uri.Connections() < bestConns {
			best = uri
		}
	}
	return best
}

type ollamaUrl struct {
	uri         url.URL
	connections int64
}

func (u *ollamaUrl) Connections() int64 {
	return atomic.LoadInt64(&u.connections)
}

func (u *ollamaUrl) Get() url.URL {
	atomic.AddInt64(&u.connections, 1)
	return u.uri
}

func (u *ollamaUrl) Done() {
	atomic.AddInt64(&u.connections, -1)
}

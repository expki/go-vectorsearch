package ai

import (
	"errors"
	"fmt"
	"math"
	"math/rand"
	"net/http"
	"net/url"
	"slices"
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

func NewOllama(cfg config.Ollama) (ai AI, err error) {
	ollama := new(Ollama)

	// Parse Ollama URI
	for _, cfgUrl := range cfg.Url {
		uriPonter, err := url.Parse(cfgUrl)
		if err != nil {
			return nil, errors.Join(fmt.Errorf("unable to parse ollama url %q", cfgUrl), err)
		} else if uriPonter == nil {
			return nil, errors.New("parsed ollama url is nil")
		}
		ollama.uri = append(ollama.uri, &ollamaUrl{
			uri: *uriPonter,
		})
	}
	ollama.token = cfg.Token

	// Create http client
	transport := &http.Transport{}
	http2.ConfigureTransport(transport)
	ollama.client = &http.Client{
		Transport: transport,
	}

	return ollama, nil
}

func (o *Ollama) Url() (uri url.URL, done func()) {
	uriList := slices.Clone(o.uri)
	rand.Shuffle(len(uriList), func(i, j int) {
		uriList[i], uriList[j] = uriList[j], uriList[i]
	})
	var best *ollamaUrl
	var bestConns int64 = math.MaxInt64
	for _, uri := range uriList {
		conns := uri.Connections()
		if conns < bestConns {
			best = uri
			bestConns = conns
		}
	}
	return best.Get(), best.Done
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

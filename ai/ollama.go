package ai

import (
	"errors"
	"fmt"
	"math"
	"math/rand"
	"net/http"
	"net/url"
	"slices"
	"sync"

	"github.com/expki/go-vectorsearch/config"
	_ "github.com/expki/go-vectorsearch/env"
	"github.com/expki/go-vectorsearch/logger"
	"golang.org/x/net/http2"
)

type Ollama struct {
	lock   sync.Mutex
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

func (o *Ollama) Url() (uri url.URL, done func()) {
	o.lock.Lock()
	uriList := slices.Clone(o.uri)
	rand.Shuffle(len(uriList), func(i, j int) {
		uriList[i], uriList[j] = uriList[j], uriList[i]
	})
	var best *ollamaUrl
	var bestConns uint64 = math.MaxUint64
	for _, uri := range uriList {
		conns := uri.Connections()
		if conns < bestConns {
			best = uri
			bestConns = conns
		}
		logger.Sugar().Debugf("Ollama %s: %d", uri.uri.String(), conns)
	}
	newconn := best.Get()
	o.lock.Unlock()
	logger.Sugar().Debugf("Ollama %s++", newconn.String())
	return newconn, func() {
		o.lock.Lock()
		best.Done()
		o.lock.Unlock()
		logger.Sugar().Debugf("Ollama %s--", newconn.String())
	}
}

type ollamaUrl struct {
	uri         url.URL
	connections uint64
}

func (u *ollamaUrl) Connections() uint64 {
	return u.connections
}

func (u *ollamaUrl) Get() url.URL {
	u.connections++
	return u.uri
}

func (u *ollamaUrl) Done() {
	u.connections--
}

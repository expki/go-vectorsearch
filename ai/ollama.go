package ai

import (
	"crypto/tls"
	"errors"
	"fmt"
	"math"
	"math/rand"
	"net"
	"net/url"
	"sync"
	"time"

	"github.com/expki/go-vectorsearch/config"
	_ "github.com/expki/go-vectorsearch/env"
)

type ai struct {
	clientManager *ClientManager
	chat          *provider
	generate      *provider
	embed         *provider
}

type provider struct {
	cfg config.Ollama

	lock        sync.Mutex
	uri         []*ollamaUrl
	token       string
	compression bool
}

func newProvider(cfg config.Ollama) (*provider, error) {
	p := &provider{
		cfg:         cfg,
		uri:         make([]*ollamaUrl, 0),
		compression: cfg.Compression,
	}
	// Parse URI
	for _, cfgUrl := range cfg.Url {
		uriPonter, err := url.Parse(cfgUrl)
		if err != nil {
			return p, errors.Join(fmt.Errorf("unable to parse ollama url %q", cfgUrl), err)
		} else if uriPonter == nil {
			return p, errors.New("parsed ollama url is nil")
		}
		p.uri = append(p.uri, &ollamaUrl{
			uri: *uriPonter,
		})
	}
	p.token = cfg.Token
	return p, nil
}

func NewAI(cfg config.AI) (a AI, err error) {
	server := new(ai)

	// Parse Embed URI
	server.embed, err = newProvider(cfg.Embed)
	if err != nil {
		return server, errors.Join(errors.New("embed config"), err)
	}

	// Parse Chat URI
	server.chat, err = newProvider(cfg.Chat)
	if err != nil {
		return server, errors.Join(errors.New("chat config"), err)
	}

	// Parse Generate URI
	server.generate, err = newProvider(cfg.Generate)
	if err != nil {
		return server, errors.Join(errors.New("generate config"), err)
	}

	// Create http client
	server.clientManager = NewClientManager(
		&tls.Config{
			InsecureSkipVerify: true,
		},
		&net.Dialer{
			Timeout:   5 * time.Second,
			KeepAlive: 30 * time.Second,
		},
	)

	return server, nil
}

func (p *provider) Url() (uri url.URL, done func()) {
	p.lock.Lock()
	defer p.lock.Unlock()
	if len(p.uri) == 0 {
		return
	}

	// randomzised order
	rand.Shuffle(len(p.uri), func(i, j int) {
		p.uri[i], p.uri[j] = p.uri[j], p.uri[i]
	})

	// find lowest connection count
	var lowestConnections *ollamaUrl
	var lowestConnectionsCount int64 = math.MaxInt64
	for _, uri := range p.uri {
		if uri.connections < lowestConnectionsCount {
			lowestConnections = uri
			lowestConnectionsCount = uri.connections
		}
	}

	// increment count
	lowestConnections.connections += 1

	// return url
	return lowestConnections.uri, func() {
		p.lock.Lock()
		lowestConnections.connections -= 1
		p.lock.Unlock()
	}
}

type ollamaUrl struct {
	uri         url.URL
	connections int64
}

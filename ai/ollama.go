package ai

import (
	"crypto/tls"
	"errors"
	"fmt"
	"math"
	"math/rand"
	"net"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/expki/go-vectorsearch/config"
	_ "github.com/expki/go-vectorsearch/env"
	"golang.org/x/net/http2"
)

type ai struct {
	client   *http.Client
	chat     *provider
	generate *provider
	embed    *provider
}

type provider struct {
	lock  sync.Mutex
	uri   []*ollamaUrl
	token string
}

func newProvider(cfg config.Ollama) (*provider, error) {
	p := &provider{uri: make([]*ollamaUrl, 0)}
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
	transport := &http.Transport{
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: true,
		},
		DialContext: (&net.Dialer{
			Timeout:   5 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		TLSHandshakeTimeout: 5 * time.Second,
		IdleConnTimeout:     90 * time.Second,
		MaxIdleConns:        50,
		MaxIdleConnsPerHost: 5,
		MaxConnsPerHost:     100,
	}
	http2.ConfigureTransport(transport)
	server.client = &http.Client{
		Transport: transport,
	}

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

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
	"slices"
	"sync/atomic"
	"time"

	"github.com/expki/go-vectorsearch/config"
	_ "github.com/expki/go-vectorsearch/env"
	"golang.org/x/net/http2"
)

type ai struct {
	client   *http.Client
	chat     provider
	generate provider
	embed    provider
}

type provider struct {
	uri   []*ollamaUrl
	token string
}

func newProvider(cfg config.Ollama) (provider, error) {
	var p provider
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

func (o *provider) Url() (uri url.URL, done func()) {
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

package httpclient

import (
	"errors"
	"fmt"
	"math"
	"math/rand/v2"
	"net/url"
	"sync"

	"github.com/expki/go-vectorsearch/config"
)

type Provider struct {
	Cfg   config.Provider
	Token string

	lock        sync.Mutex
	uri         []*httpurl
	compression bool
}

func NewProvider(cfg config.Provider) (provider *Provider, err error) {
	provider = &Provider{
		Cfg:         cfg,
		uri:         make([]*httpurl, 0),
		compression: cfg.RequestCompression,
	}
	// Parse URI
	for _, cfgUrl := range cfg.ApiBase {
		uriPonter, err := url.Parse(cfgUrl)
		if err != nil {
			return provider, errors.Join(fmt.Errorf("unable to parse ollama url %q", cfgUrl), err)
		} else if uriPonter == nil {
			return provider, errors.New("parsed ollama url is nil")
		}
		provider.uri = append(provider.uri, &httpurl{
			uri: *uriPonter,
		})
	}
	provider.Token = cfg.ApiKey
	return provider, nil
}

type httpurl struct {
	uri         url.URL
	connections int64
}

func (p *Provider) Url() (uri url.URL, done func()) {
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
	var lowestConnections *httpurl
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

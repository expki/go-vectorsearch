package ai

import (
	"context"
	"crypto/tls"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/expki/go-vectorsearch/config"
	"github.com/expki/go-vectorsearch/logger"
	"golang.org/x/net/http2"
)

func NewClientManager(tlsClientConfig *tls.Config, dialer *net.Dialer) *ClientManager {
	return &ClientManager{
		tlsClientConfig: tlsClientConfig,
		dialer:          dialer,
		clients:         make(map[string]*clientInstance),
	}
}

type clientInstance struct {
	address string
	client  *http.Client

	lockCloser sync.Mutex
	close      func() error

	lockRequests   sync.Mutex
	activeRequests int64
	totalRequests  uint64
}

type ClientManager struct {
	tlsClientConfig *tls.Config
	dialer          *net.Dialer

	lock    sync.RWMutex
	clients map[string]*clientInstance
}

func (m *ClientManager) GetHttpClient(address string) (client *http.Client, close func()) {
	// return current client
	m.lock.RLock()
	c, ok := m.clients[address]
	m.lock.RUnlock()
	if ok {
		c.lockRequests.Lock()
		if c.totalRequests < config.HTTP_CLIENT_MAX_REQUESTS {
			c.totalRequests++
			c.activeRequests++
			c.lockRequests.Unlock()
			return c.client, c.Close
		}
		c.lockRequests.Unlock()
	}
	c = &clientInstance{}

	// create new client
	transport := &http.Transport{
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			logger.Sugar().Debugf("opening http tcp client: %s", address)
			conn, err := m.dialer.DialContext(ctx, network, addr)
			if err != nil {
				return nil, err
			}
			c.lockCloser.Lock()
			c.close = conn.Close
			c.lockCloser.Unlock()
			return conn, nil
		},
		DialTLSContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			logger.Sugar().Debugf("opening http tls client: %s", address)
			// TCP connect
			conn, err := m.dialer.DialContext(ctx, network, addr)
			if err != nil {
				return nil, err
			}
			c.lockCloser.Lock()
			c.close = conn.Close
			c.lockCloser.Unlock()

			// TLS upgrade
			tlsConn := tls.Client(conn, m.tlsClientConfig)
			if err := tlsConn.HandshakeContext(ctx); err != nil {
				conn.Close()
				return nil, err
			}

			return tlsConn, nil
		},
		IdleConnTimeout:     90 * time.Second,
		MaxIdleConns:        50,
		MaxIdleConnsPerHost: 50,
		MaxConnsPerHost:     100,
	}
	http2.ConfigureTransport(transport)
	c = &clientInstance{
		address: address,
		client: &http.Client{
			Transport: transport,
		},
		activeRequests: 1,
		totalRequests:  1,
	}

	// save client
	m.lock.Lock()
	m.clients[address] = c
	m.lock.Unlock()

	// return new client
	return c.client, c.Close
}

func (c *clientInstance) Close() {
	c.lockRequests.Lock()
	c.activeRequests--
	if c.activeRequests <= 0 && c.totalRequests >= config.HTTP_CLIENT_MAX_REQUESTS {
		c.lockCloser.Lock()
		logger.Sugar().Debugf("closing http client: %s", c.address)
		if c.close != nil {
			c.close()
		}
		c.lockCloser.Unlock()
	}
	c.lockRequests.Unlock()
	return
}

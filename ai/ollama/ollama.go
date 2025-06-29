package ollama

import (
	"crypto/tls"
	"errors"
	"net"
	"time"

	"github.com/expki/go-vectorsearch/ai/httpclient"
	"github.com/expki/go-vectorsearch/config"
	"github.com/expki/go-vectorsearch/logger"
)

type Ollama struct {
	Cfg           config.AI
	clientManager *httpclient.ClientManager
	chat          *httpclient.Provider
	generate      *httpclient.Provider
	embed         *httpclient.Provider
}

func New(cfg config.AI) (ai *Ollama, err error) {
	ai = new(Ollama)
	ai.Cfg = cfg

	// Parse Embed URI
	if cfg.Embed != nil {
		logger.Sugar().Info("Adding AI provider: Ollama-Embed")
		ai.embed, err = httpclient.NewProvider(*cfg.Embed)
		if err != nil {
			return ai, errors.Join(errors.New("embed config"), err)
		}
	}

	// Parse Chat URI
	if cfg.Chat != nil {
		logger.Sugar().Info("Adding AI provider: Ollama-Chat")
		ai.chat, err = httpclient.NewProvider(*cfg.Chat)
		if err != nil {
			return ai, errors.Join(errors.New("chat config"), err)
		}
	}

	// Parse Generate URI
	if cfg.Generate != nil {
		logger.Sugar().Info("Adding AI provider: Ollama-Generate")
		ai.generate, err = httpclient.NewProvider(*cfg.Generate)
		if err != nil {
			return ai, errors.Join(errors.New("generate config"), err)
		}
	}

	// Create http client
	ai.clientManager = httpclient.NewClientManager(
		&tls.Config{
			InsecureSkipVerify: true,
		},
		&net.Dialer{
			Timeout:   5 * time.Second,
			KeepAlive: 30 * time.Second,
		},
	)

	return ai, nil
}

func (ai *Ollama) CanEmbed() bool {
	return ai.embed != nil
}

func (ai *Ollama) CanChat() bool {
	return ai.chat != nil
}

func (ai *Ollama) CanGenerate() bool {
	return ai.generate != nil
}

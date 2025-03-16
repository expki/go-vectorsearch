package server

import (
	"sync/atomic"

	"github.com/expki/go-vectorsearch/ai"
	"github.com/expki/go-vectorsearch/config"
	"github.com/expki/go-vectorsearch/database"
	_ "github.com/expki/go-vectorsearch/env"
)

var index atomic.Uint64

type server struct {
	db     *database.Database
	ai     ai.AI
	config config.Ollama
}

func New(cfg config.Ollama, db *database.Database, ai ai.AI) *server {
	return &server{
		db:     db,
		ai:     ai,
		config: cfg,
	}
}

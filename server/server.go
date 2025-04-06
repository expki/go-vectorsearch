package server

import (
	"sync/atomic"

	"github.com/expki/go-vectorsearch/ai"
	"github.com/expki/go-vectorsearch/config"
	"github.com/expki/go-vectorsearch/database"
	_ "github.com/expki/go-vectorsearch/env"
)

var index atomic.Uint64

func New(cfg config.AI, db *database.Database, ai ai.AI) *Server {
	return &Server{
		db:     db,
		ai:     ai,
		config: cfg,
	}
}

type Server struct {
	db     *database.Database
	ai     ai.AI
	config config.AI
}

package server

import (
	"context"
	"sync/atomic"

	"github.com/expki/go-vectorsearch/ai"
	"github.com/expki/go-vectorsearch/cache"
	"github.com/expki/go-vectorsearch/config"
	"github.com/expki/go-vectorsearch/database"
	_ "github.com/expki/go-vectorsearch/env"
)

var index atomic.Uint64

func New(appCtx context.Context, cfg config.AI, db *database.Database, ai ai.AI) *Server {
	return &Server{
		db:     db,
		ai:     ai,
		config: cfg,
		cache:  cache.NewCache(appCtx),
	}
}

type Server struct {
	db     *database.Database
	ai     ai.AI
	config config.AI
	cache  *cache.Cache
}

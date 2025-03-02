package server

import (
	"sync/atomic"

	"github.com/expki/go-vectorsearch/ai"
	"github.com/expki/go-vectorsearch/config"
	_ "github.com/expki/go-vectorsearch/env"
	"gorm.io/gorm"
)

var index atomic.Uint64

type server struct {
	db     *gorm.DB
	ai     *ai.Ollama
	config config.Config
}

func New(cfg config.Config, db *gorm.DB, ai *ai.Ollama) *server {
	return &server{
		db:     db,
		ai:     ai,
		config: cfg,
	}
}

package server

import (
	"sync/atomic"

	"github.com/expki/govecdb/ai"
	"github.com/expki/govecdb/config"
	_ "github.com/expki/govecdb/env"
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

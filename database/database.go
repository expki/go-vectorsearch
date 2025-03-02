package database

import (
	"errors"
	"fmt"

	"github.com/expki/go-vectorsearch/config"
	_ "github.com/expki/go-vectorsearch/env"
	"github.com/expki/go-vectorsearch/logger"
	"gorm.io/gorm"
	"gorm.io/plugin/dbresolver"
)

func New(cfg config.Database) (db *gorm.DB, err error) {
	// get dialectors from config
	readwrite, readonly := cfg.GetDialectors()
	if len(readwrite) == 0 {
		return nil, errors.New("no writable database configured")
	}

	// open primary database connection
	db, err = gorm.Open(readwrite[0], &gorm.Config{
		SkipDefaultTransaction: true,
		PrepareStmt:            true,
	})
	if err != nil {
		logger.Sugar().Debugf("config: %+v", cfg)
		logger.Sugar().Debugf("dsn: %+v", readwrite[0])
		return nil, fmt.Errorf("failed to open database connection: %v", err)
	}
	db.Clauses(dbresolver.Write).AutoMigrate(
		&Document{},
		&Embedding{},
	)

	// add resolver connections
	if len(readonly)+len(readwrite) > 1 {
		logger.Sugar().Debugf("Enabling database resolver for read/write splitting. Sources: %d, Replicas: %d", len(readwrite), len(readonly))
		err = db.Use(dbresolver.Register(dbresolver.Config{
			Sources:           readwrite,
			Replicas:          readonly,
			Policy:            dbresolver.StrictRoundRobinPolicy(),
			TraceResolverMode: true,
		}))
		if err != nil {
			logger.Sugar().Errorf("failed to register database resolver: %v", err)
			return nil, err
		}
	}
	return db, nil
}

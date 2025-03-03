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

type Database struct {
	*gorm.DB
	Cache *Cache
}

func New(cfg config.Database, vectorSize int) (db *Database, err error) {
	// get dialectors from config
	readwrite, readonly := cfg.GetDialectors()
	if len(readwrite) == 0 {
		return nil, errors.New("no writable database configured")
	}

	// open primary database connection
	godb, err := gorm.Open(readwrite[0], &gorm.Config{
		SkipDefaultTransaction: true,
		PrepareStmt:            true,
	})
	if err != nil {
		logger.Sugar().Debugf("config: %+v", cfg)
		logger.Sugar().Debugf("dsn: %+v", readwrite[0])
		return nil, fmt.Errorf("failed to open database connection: %v", err)
	}
	godb.Clauses(dbresolver.Write).AutoMigrate(
		&Document{},
	)

	// add resolver connections
	if len(readonly)+len(readwrite) > 1 {
		logger.Sugar().Debugf("Enabling database resolver for read/write splitting. Sources: %d, Replicas: %d", len(readwrite), len(readonly))
		err = godb.Use(dbresolver.Register(dbresolver.Config{
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

	// open cache
	cache, err := NewCache(cfg.Cache, vectorSize)
	if err != nil {
		logger.Sugar().Errorf("failed to load cache: %v", err)
		return nil, err
	}

	return &Database{DB: godb, Cache: cache}, nil
}

func (d *Database) Close() error {
	db, err := d.DB.DB()
	if err != nil {
		logger.Sugar().Errorf("failed to get database connection: %v", err)
		return err
	}
	err = db.Close()
	if err != nil {
		logger.Sugar().Errorf("failed to close database connection: %v", err)
		return err
	}
	return nil
}

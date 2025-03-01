package database

import (
	"errors"

	"github.com/expki/govecdb/config"
	_ "github.com/expki/govecdb/env"
	"github.com/expki/govecdb/logger"
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
		panic("failed to connect database")
	}
	db.Clauses(dbresolver.Write).AutoMigrate(
		&Document{},
		&Embedding{},
	)

	// add resolver connections
	if len(readonly)+len(readwrite) > 1 {
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

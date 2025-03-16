package database

import (
	"context"
	"errors"
	"log"
	"os"
	"time"

	"github.com/expki/go-vectorsearch/config"
	_ "github.com/expki/go-vectorsearch/env"
	"github.com/expki/go-vectorsearch/logger"
	"gorm.io/gorm"
	glog "gorm.io/gorm/logger"
	"gorm.io/plugin/dbresolver"
)

type Database struct {
	cfg config.Database
	*gorm.DB
}

func New(appCtx context.Context, cfg config.Database) (db *Database, err error) {
	// create logger
	glogger := glog.New(log.New(os.Stdout, "\r\n", log.LstdFlags), glog.Config{
		SlowThreshold:             30 * time.Second,
		LogLevel:                  cfg.LogLevel.GORM(),
		IgnoreRecordNotFoundError: true,
		Colorful:                  true,
	})

	// get dialectors from config
	readwrite, readonly := cfg.GetDialectors()
	if len(readwrite) == 0 {
		return nil, errors.New("no writable database configured")
	}

	// open primary database connection
	godb, err := gorm.Open(readwrite[0], &gorm.Config{
		SkipDefaultTransaction: true,
		PrepareStmt:            true,
		Logger:                 glogger,
	})
	if err != nil {
		logger.Sugar().Debugf("config: %+v", cfg)
		logger.Sugar().Debugf("dsn: %+v", readwrite[0])
		return nil, errors.Join(errors.New("failed to open database connection"), err)
	}
	if sqldb, err := godb.DB(); err == nil {
		sqldb.SetConnMaxIdleTime(5 * time.Minute)
		sqldb.SetConnMaxLifetime(time.Hour)
		sqldb.SetMaxIdleConns(5)
		sqldb.SetMaxOpenConns(10)
	}
	godb.Clauses(dbresolver.Write).AutoMigrate(
		&Owner{},
		&Category{},
		&Centroid{},
		&Document{},
	)

	// add resolver connections
	if len(readonly)+len(readwrite) > 1 {
		logger.Sugar().Debugf("Enabling database resolver for read/write splitting. Sources: %d, Replicas: %d", len(readwrite), len(readonly))
		err = godb.Use(
			dbresolver.Register(dbresolver.Config{
				Sources:           readwrite,
				Replicas:          readonly,
				Policy:            dbresolver.StrictRoundRobinPolicy(),
				TraceResolverMode: true,
			}).
				SetConnMaxIdleTime(5 * time.Minute).
				SetConnMaxLifetime(time.Hour).
				SetMaxIdleConns(5).
				SetMaxOpenConns(10))
		if err != nil {
			logger.Sugar().Errorf("failed to register database resolver: %v", err)
			return nil, err
		}
	}
	db = &Database{cfg: cfg, DB: godb}
	go db.refreshCentroidJob(appCtx)

	return db, nil
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

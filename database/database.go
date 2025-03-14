package database

import (
	"errors"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/expki/go-vectorsearch/config"
	_ "github.com/expki/go-vectorsearch/env"
	"github.com/expki/go-vectorsearch/logger"
	"gorm.io/gorm"
	glog "gorm.io/gorm/logger"
	"gorm.io/plugin/dbresolver"
)

type Database struct {
	*gorm.DB
	Cache Cache
}

func New(cfg config.Config, vectorSize int) (db *Database, err error) {
	// create logger
	glogger := glog.New(log.New(os.Stdout, "\r\n", log.LstdFlags), glog.Config{
		SlowThreshold:             30 * time.Second,
		LogLevel:                  cfg.Database.LogLevel.GORM(),
		IgnoreRecordNotFoundError: false,
		Colorful:                  true,
	})

	// get dialectors from config
	readwrite, readonly := cfg.Database.GetDialectors()
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
	db = &Database{DB: godb, Cache: Cache{path: cfg.Cache, vectorSize: vectorSize}}

	// create cache dir
	if _, err := os.Stat(cfg.Cache); os.IsNotExist(err) {
		err = os.Mkdir(cfg.Cache, 0755)
		if err != nil {
			logger.Sugar().Errorf("failed to create cache dir %q: %v", cfg.Cache, err)
			return nil, err
		}
	}

	// open cache
	files, err := os.ReadDir(cfg.Cache)
	if err != nil {
		logger.Sugar().Errorf("failed to read cache dir %q: %v", cfg.Cache, err)
		return nil, err
	}
	for _, file := range files {
		if file.IsDir() {
			continue
		}
		info, err := file.Info()
		if err != nil {
			logger.Sugar().Errorf("failed to get file info for %q: %v", file.Name(), err)
			continue
		}
		if !strings.HasPrefix(info.Name(), "centroid_") || !strings.HasPrefix(info.Name(), ".cache") {
			continue
		}
		indexString, _ := strings.CutPrefix(info.Name(), "centroid_")
		indexString, _ = strings.CutSuffix(indexString, ".cache")
		index, err := strconv.Atoi(indexString)
		if err != nil {
			logger.Sugar().Errorf("failed to parse index from file name %q: %v", info.Name(), err)
			continue
		}
		filePath := filepath.Join(cfg.Cache, file.Name())
		file, err := os.OpenFile(filePath, os.O_RDWR|os.O_CREATE, 0644)
		if err != nil {
			logger.Sugar().Errorf("failed to open cache file %q: %v", filePath, err)
			return nil, err
		}
		centroid := &centroid{Idx: index, file: file}
		db.Cache.centroids = append(db.Cache.centroids, centroid)
	}
	sort.Slice(db.Cache.centroids, func(i, j int) bool {
		return db.Cache.centroids[i].Idx < db.Cache.centroids[j].Idx // ascending order
	})

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
	d.Cache.Close()
	return nil
}

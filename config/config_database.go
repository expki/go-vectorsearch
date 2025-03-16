package config

import (
	"encoding/json"
	"strings"

	_ "github.com/expki/go-vectorsearch/env"
	"gorm.io/driver/postgres"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type Database struct {
	Sqlite           string                `json:"sqlite"`
	Postgres         SingleOrSlice[string] `json:"postgres"`
	PostgresReadOnly SingleOrSlice[string] `json:"postgres_readonly"`
	LogLevel         LogLevel              `json:"log_level"` // 0: Silent, 1: Error, 2: Warn, 3: Info, 4: Debug
}

func (c Database) GetDialectors() (readwrite, readonly []gorm.Dialector) {
	if c.Sqlite != "" {
		readwrite = append(readwrite, sqlite.Open(c.Sqlite))
		return
	}
	for _, dsn := range c.Postgres {
		if dsn != "" {
			readwrite = append(readwrite, postgres.Open(dsn))
		}
	}
	for _, dsn := range c.PostgresReadOnly {
		if dsn != "" {
			readonly = append(readonly, postgres.Open(dsn))
		}
	}
	return
}

func (c LogLevel) GORM() (level logger.LogLevel) {
	switch strings.ToLower(strings.TrimSpace(c.String())) {
	case LogLevelDebug.String(), "trace":
		return logger.Info
	case LogLevelInfo.String(), "information", "notice":
		return logger.Info
	case LogLevelWarn.String(), "warning", "alert":
		return logger.Warn
	case LogLevelError.String():
		return logger.Error
	case LogLevelFatal.String(), "critical", "emergency":
		return logger.Error
	case LogLevelPanic.String():
		return logger.Error
	case "silent":
		return logger.Silent
	default:
		return logger.Error
	}
}

// SingleOrSlice allows for a configuration field to be either a single value or a slice of values.
type SingleOrSlice[T any] []T

// UnmarshalJSON handles both single values and slices for the field.
func (s *SingleOrSlice[T]) UnmarshalJSON(data []byte) error {
	var single T
	if err := json.Unmarshal(data, &single); err == nil {
		*s = SingleOrSlice[T]{single}
		return nil
	}
	var slice []T
	if err := json.Unmarshal(data, &slice); err != nil {
		return err
	}
	*s = slice
	return nil
}

// MarshalJSON ensures that the field is marshaled correctly whether it's a single value or a slice.
func (s SingleOrSlice[T]) MarshalJSON() ([]byte, error) {
	if len(s) == 1 {
		return json.Marshal(s[0])
	}
	return json.Marshal([]T(s))
}

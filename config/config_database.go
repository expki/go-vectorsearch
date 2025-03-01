package config

import (
	"encoding/json"

	_ "github.com/expki/govecdb/env"
	"gorm.io/driver/postgres"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

type Database struct {
	Sqlite           string                `json:"sqlite"`
	Postgres         SingleOrSlice[string] `json:"postgres"`
	PostgresReadOnly SingleOrSlice[string] `json:"postgres_readonly"`
}

func (c Database) GetDialectors() (readwrite, readonly []gorm.Dialector) {
	if c.Sqlite != "" {
		readwrite = append(readwrite, sqlite.Open(c.Sqlite))
		return
	}
	for _, dsn := range c.Postgres {
		readwrite = append(readwrite, postgres.Open(dsn))
	}
	for _, dsn := range c.PostgresReadOnly {
		readonly = append(readonly, postgres.Open(dsn))
	}
	return
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

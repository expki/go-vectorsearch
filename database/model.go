package database

import (
	"time"

	_ "github.com/expki/go-vectorsearch/env"
)

type Document struct {
	ID        uint64    `gorm:"primarykey"`
	UpdatedAt time.Time `gorm:"autoUpdateTime"`
	Prefix    string
	Document  DocumentField
	Vector    VectorField
}

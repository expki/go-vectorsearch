package database

import (
	"time"

	_ "github.com/expki/govecdb/env"
)

type Document struct {
	ID        uint      `gorm:"primarykey"`
	UpdatedAt time.Time `gorm:"autoUpdateTime"`
	Prefix    string
	Document  DocumentField
}

type Embedding struct {
	ID     uint `gorm:"primarykey"`
	Vector VectorField

	DocumentID uint     `gorm:"index"`
	Document   Document `gorm:"foreignKey:DocumentID;references:ID"`
}

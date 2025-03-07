package database

import (
	"time"

	_ "github.com/expki/go-vectorsearch/env"
	"gorm.io/gorm"
)

type Document struct {
	ID        uint64        `gorm:"primarykey"`
	Vector    VectorField   `gorm:"not null"`
	UpdatedAt time.Time     `gorm:"autoUpdateTime;index;not null"`
	Prefix    string        `gorm:"not null"`
	Document  DocumentField `gorm:"not null"`
	Hash      string        `gorm:"uniqueIndex;not null"`
}

func (m *Document) BeforeCreate(tx *gorm.DB) error {
	m.UpdatedAt = time.Now().UTC()
	return nil
}

func (m *Document) BeforeUpdate(tx *gorm.DB) error {
	m.UpdatedAt = time.Now().UTC()
	return nil
}

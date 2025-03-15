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
	Hash      string        `gorm:"uniqueIndex:uq_document_hash;not null"`

	// Parent
	CategoryID uint64   `gorm:"uniqueIndex:uq_document_hash;index:idx_document_category;not null"`
	Category   Category `gorm:"foreignKey:CategoryID;constraint:onUpdate:CASCADE,onDelete:CASCADE"`
}

func (m *Document) BeforeCreate(tx *gorm.DB) error {
	m.UpdatedAt = time.Now().UTC()
	return nil
}

func (m *Document) BeforeUpdate(tx *gorm.DB) error {
	m.UpdatedAt = time.Now().UTC()
	return nil
}

type Category struct {
	ID   uint64 `gorm:"primarykey"`
	Name string `gorm:"index:uq_category_name,unique;not null"`

	// Parent
	OwnerID uint64 `gorm:"index:uq_category_name,unique;not null"`
	Owner   Owner  `gorm:"foreignKey:OwnerID;constraint:onUpdate:CASCADE,onDelete:CASCADE"`

	// Children
	Documents []Document `gorm:"foreignKey:CategoryID;constraint:onUpdate:CASCADE,onDelete:CASCADE"`
}

type Owner struct {
	ID   uint64 `gorm:"primarykey"`
	Name string `gorm:"uniqueIndex;not null"`

	// Children
	Categories []Category `gorm:"foreignKey:OwnerID;constraint:onUpdate:CASCADE,onDelete:CASCADE"`
}

package database

import (
	"time"

	_ "github.com/expki/go-vectorsearch/env"
)

type Embedding struct {
	ID     uint64 `gorm:"primarykey"`
	Vector []byte `gorm:"not null"`

	// Parent
	DocumentID uint64    `gorm:"index:idx_embedding_document;not null"`
	Document   *Document `gorm:"foreignKey:DocumentID"`
	CentroidID uint64    `gorm:"index:idx_embedding_centroid;not null"`
	Centroid   *Centroid `gorm:"foreignKey:CentroidID"`
}

type Document struct {
	ID          uint64        `gorm:"primarykey"`
	Name        string        `gorm:"not null"`
	ExternalID  string        `gorm:"not null"`
	LastUpdated time.Time     `gorm:"index:idx_document_updated;not null"`
	Document    DocumentField `gorm:"not null"`

	// Parent
	CategoryID uint64    `gorm:"not null"`
	Category   *Category `gorm:"foreignKey:CategoryID"`

	// Children
	Embeddings []*Embedding `gorm:"foreignKey:DocumentID;constraint:onUpdate:CASCADE,onDelete:CASCADE"`
}

type Centroid struct {
	ID          uint64    `gorm:"primarykey"`
	Vector      []byte    `gorm:"not null"`
	LastUpdated time.Time `gorm:"index:idx_centroid_updated;not null"`

	// Parent
	CategoryID uint64    `gorm:"index:idx_centroid_category;not null"`
	Category   *Category `gorm:"foreignKey:CategoryID"`

	// Children
	Embeddings []*Embedding `gorm:"foreignKey:CentroidID;constraint:onUpdate:CASCADE,onDelete:CASCADE"`
}

type Category struct {
	ID   uint64 `gorm:"primarykey"`
	Name string `gorm:"uniqueIndex:uq_category_name;not null"`

	// Parent
	OwnerID uint64 `gorm:"uniqueIndex:uq_category_name;not null"`
	Owner   *Owner `gorm:"foreignKey:OwnerID"`

	// Children
	Centroids []*Centroid `gorm:"foreignKey:CategoryID;constraint:onUpdate:CASCADE,onDelete:CASCADE"`
	Documents []*Document `gorm:"foreignKey:CategoryID;constraint:onUpdate:CASCADE,onDelete:CASCADE"`
}

type Owner struct {
	ID   uint64 `gorm:"primarykey"`
	Name string `gorm:"uniqueIndex:uq_owner_name;not null"`

	// Children
	Categories []*Category `gorm:"foreignKey:OwnerID;constraint:onUpdate:CASCADE,onDelete:CASCADE"`
}

package server

import (
	"fmt"
	"strings"
)

func newUploading(title string, bodySections []string) (item uploading) {
	// define title
	if value := strings.TrimSpace(title); value != "" {
		item.title.value = fmt.Sprintf("search_document: %s", value)
		item.title.enabled = true
	}
	// define document
	item.body = make([]struct {
		section   string
		embedding []byte
	}, 0, len(bodySections))
	for _, section := range bodySections {
		if value := strings.TrimSpace(section); value != "" {
			item.body = append(item.body, struct {
				section   string
				embedding []byte
			}{
				section: fmt.Sprintf("search_document: %s", section),
			})
		}
	}
	return item
}

type uploading struct {
	title struct {
		value     string
		embedding []byte
		enabled   bool
	}
	body []struct {
		section   string
		embedding []byte
	}
}

func (u uploading) CountEmbeddings() (count int) {
	if u.title.enabled {
		count++
	}
	count += len(u.body)
	return count
}

type uploadingList []uploading

func (ul uploadingList) Sections() (sections []string) {
	sections = make([]string, 0, len(ul))
	for _, item := range ul {
		if item.title.enabled {
			sections = append(sections, item.title.value)
		}
		for _, section := range item.body {
			sections = append(sections, section.section)
		}
	}
	return sections
}

func (ul uploadingList) CountEmbeddings() (count int) {
	for _, item := range ul {
		count += item.CountEmbeddings()
	}
	return count
}

func (ul uploadingList) TakeEmbedding() []byte {
	for _, item := range ul {
		if item.title.enabled {
			return item.title.embedding
		}
		for _, section := range item.body {
			return section.embedding
		}
	}
	return nil
}

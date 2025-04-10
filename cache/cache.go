package cache

import (
	"context"
	"sync"
	"time"

	"github.com/expki/go-vectorsearch/config"
	"github.com/expki/go-vectorsearch/database"
)

func NewCache(appCtx context.Context) *Cache {
	c := &Cache{
		done:      make(chan struct{}),
		owner:     make(map[string]*item[database.Owner]),
		category:  make(map[string]*item[database.Category]),
		centroids: make(map[string]*item[[]database.Centroid]),
	}
	go c.cleanupTask(appCtx)
	return c
}

func (c *Cache) Close() {
	close(c.done)
}

type Cache struct {
	done chan struct{}

	ownerLock     sync.RWMutex
	owner         map[string]*item[database.Owner]
	categoryLock  sync.RWMutex
	category      map[string]*item[database.Category]
	centroidsLock sync.RWMutex
	centroids     map[string]*item[[]database.Centroid]
}

func (c *Cache) cleanupTask(appCtx context.Context) {
	ticker := time.NewTicker(config.CACHE_CLEANUP)
	for {
		select {
		case <-appCtx.Done():
			ticker.Stop()
			return
		case <-c.done:
			ticker.Stop()
			return
		case <-ticker.C:
			now := time.Now()

			// Cleanup owner
			c.ownerLock.Lock()
			for key, value := range c.owner {
				if value.expiration.Before(now) {
					delete(c.owner, key)
				}
			}
			c.ownerLock.Unlock()

			// Cleanup category
			c.ownerLock.Lock()
			for key, value := range c.category {
				if value.expiration.Before(now) {
					delete(c.category, key)
				}
			}
			c.ownerLock.Unlock()

			// Cleanup centroids
			c.ownerLock.Lock()
			for key, value := range c.centroids {
				if value.expiration.Before(now) {
					delete(c.centroids, key)
				}
			}
			c.ownerLock.Unlock()
		}
	}
}

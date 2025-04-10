package cache

import (
	"errors"
	"time"

	"github.com/expki/go-vectorsearch/config"
	"github.com/expki/go-vectorsearch/database"
	"golang.org/x/sync/singleflight"
)

var (
	ownerSingleflight     singleflight.Group
	categorySingleflight  singleflight.Group
	centroidsSingleflight singleflight.Group
)

func (c *Cache) FetchOwner(name string, fetch func() (database.Owner, error)) (value database.Owner, err error) {
	key := ownerKey{Name: name}.String()

	// singleflight fetch
	valueAny, err, _ := centroidsSingleflight.Do(key, func() (any, error) {
		// retrieve cache item
		c.ownerLock.RLock()
		cacheValue, ok := c.owner[key]
		c.ownerLock.RUnlock()

		// check cache value
		valid := false
		if ok && cacheValue.expiration.After(time.Now()) {
			value = cacheValue.value
			valid = true
		}

		// return cache value if valid
		if valid {
			return value, nil
		}

		// fetch new result
		values, err := fetch()
		if err != nil {
			return values, err
		}

		// save new result
		c.ownerLock.Lock()
		c.owner[key] = &item[database.Owner]{
			expiration: time.Now().Add(config.CACHE_DURATION),
			value:      value,
		}
		c.ownerLock.Unlock()

		// return new result
		return values, err
	})
	if err != nil {
		return value, err
	}
	value, ok := valueAny.(database.Owner)
	if !ok {
		return value, errors.New("failed to cast singleflight response value to type")
	}
	return value, err
}

func (c *Cache) FetchCategory(name string, ownerID uint64, fetch func() (database.Category, error)) (value database.Category, err error) {
	key := categoryKey{Name: name, OwnerID: ownerID}.String()

	// singleflight fetch
	valueAny, err, _ := centroidsSingleflight.Do(key, func() (any, error) {
		// retrieve cache item
		c.categoryLock.RLock()
		cacheValue, ok := c.category[key]
		c.categoryLock.RUnlock()

		// check cache value
		valid := false
		if ok && cacheValue.expiration.After(time.Now()) {
			value = cacheValue.value
			valid = true
		}

		// return cache value if valid
		if valid {
			return value, nil
		}

		// fetch new result
		values, err := fetch()
		if err != nil {
			return values, err
		}

		// save new result
		c.categoryLock.Lock()
		c.category[key] = &item[database.Category]{
			expiration: time.Now().Add(config.CACHE_DURATION),
			value:      value,
		}
		c.categoryLock.Unlock()

		// return new result
		return values, err
	})
	if err != nil {
		return value, err
	}
	value, ok := valueAny.(database.Category)
	if !ok {
		return value, errors.New("failed to cast singleflight response value to type")
	}
	return value, err
}

func (c *Cache) FetchCentroids(categoryID uint64, fetch func() ([]database.Centroid, error)) (values []database.Centroid, err error) {
	key := centroidsKey{CategoryID: categoryID}.String()

	// singleflight fetch
	valueAny, err, _ := centroidsSingleflight.Do(key, func() (any, error) {
		// retrieve cache item
		c.centroidsLock.RLock()
		cacheValue, ok := c.centroids[key]
		c.centroidsLock.RUnlock()

		// check cache value
		valid := false
		if ok && cacheValue.expiration.After(time.Now()) {
			values = cacheValue.value
			valid = true
		}

		// return cache value if valid
		if valid {
			return values, nil
		}

		// fetch new result
		values, err := fetch()
		if err != nil {
			return values, err
		}

		// save new result
		c.centroidsLock.Lock()
		c.centroids[key] = &item[[]database.Centroid]{
			expiration: time.Now().Add(config.CACHE_DURATION),
			value:      values,
		}
		c.centroidsLock.Unlock()

		// return new result
		return values, err
	})
	if err != nil {
		return values, err
	}
	values, ok := valueAny.([]database.Centroid)
	if !ok {
		return values, errors.New("failed to cast singleflight response value to type")
	}
	return values, err
}

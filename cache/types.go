package cache

import (
	"fmt"
	"strconv"
	"time"
)

type ownerKey struct {
	Name string
}

func (k ownerKey) String() string {
	return k.Name
}

type categoryKey struct {
	Name    string
	OwnerID uint64
}

func (k categoryKey) String() string {
	return fmt.Sprintf("%d:%s", k.OwnerID, k.Name)
}

type centroidsKey struct {
	CategoryID uint64
}

func (k centroidsKey) String() string {
	return strconv.FormatUint(k.CategoryID, 10)
}

type item[T any] struct {
	expiration time.Time
	value      T
}

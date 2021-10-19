package routing

import (
	"sync"

	"github.com/iskorotkov/router/internal/models"
)

type RouteInfo struct {
	To   string
	Type models.RouteType
}

type Cache struct {
	routes map[string]RouteInfo
	m      sync.RWMutex
}

func New() Cache {
	return Cache{
		routes: make(map[string]RouteInfo),
		m:      sync.RWMutex{},
	}
}

func (c *Cache) Get(key string) (RouteInfo, bool) {
	c.m.RLock()
	defer c.m.RUnlock()

	value, ok := c.routes[key]

	return value, ok
}

func (c *Cache) GetAll() map[string]RouteInfo {
	c.m.RLock()
	defer c.m.RUnlock()

	result := make(map[string]RouteInfo)

	for key, value := range c.routes {
		result[key] = value
	}

	return result
}

func (c *Cache) Set(key string, value RouteInfo) {
	c.m.Lock()
	defer c.m.Unlock()

	c.routes[key] = value
}

func (c *Cache) Exists(key string) bool {
	c.m.RLock()
	defer c.m.RUnlock()

	_, ok := c.routes[key]

	return ok
}

func (c *Cache) Remove(key string) {
	c.m.Lock()
	defer c.m.Unlock()

	delete(c.routes, key)
}

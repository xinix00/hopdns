package dns

import (
	"net"
	"sync"
)

// Cache stores job name -> IPs mapping
type Cache struct {
	mu    sync.RWMutex
	data  map[string][]net.IP
}

// NewCache creates a new cache
func NewCache() *Cache {
	return &Cache{
		data: make(map[string][]net.IP),
	}
}

// Get returns IPs for a job name
func (c *Cache) Get(jobName string) []net.IP {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.data[jobName]
}

// Set stores IPs for a job name
func (c *Cache) Set(jobName string, ips []net.IP) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.data[jobName] = ips
}

// Update replaces entire cache with new data
func (c *Cache) Update(data map[string][]net.IP) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.data = data
}

// GetAll returns all cached data
func (c *Cache) GetAll() map[string][]net.IP {
	c.mu.RLock()
	defer c.mu.RUnlock()

	result := make(map[string][]net.IP)
	for k, v := range c.data {
		result[k] = v
	}
	return result
}

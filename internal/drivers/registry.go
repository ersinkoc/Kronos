package drivers

import (
	"fmt"
	"sort"
	"sync"
)

// Registry stores available driver implementations.
type Registry struct {
	mu      sync.RWMutex
	drivers map[string]Driver
}

// NewRegistry returns an empty driver registry.
func NewRegistry() *Registry {
	return &Registry{drivers: make(map[string]Driver)}
}

// Register adds driver to the registry.
func (r *Registry) Register(driver Driver) error {
	if driver == nil {
		return fmt.Errorf("driver is required")
	}
	name := driver.Name()
	if name == "" {
		return fmt.Errorf("driver name is required")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.drivers[name]; exists {
		return fmt.Errorf("driver %q already registered", name)
	}
	r.drivers[name] = driver
	return nil
}

// Get returns a registered driver by name.
func (r *Registry) Get(name string) (Driver, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	driver, ok := r.drivers[name]
	return driver, ok
}

// Names returns registered driver names in sorted order.
func (r *Registry) Names() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	names := make([]string, 0, len(r.drivers))
	for name := range r.drivers {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

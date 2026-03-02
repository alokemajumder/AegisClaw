package connectorsdk

import (
	"fmt"
	"sync"
)

// Factory creates a new connector instance.
type Factory func() Connector

// Registry manages available connector types.
type Registry struct {
	mu        sync.RWMutex
	factories map[string]Factory
}

// NewRegistry creates a new connector registry.
func NewRegistry() *Registry {
	return &Registry{
		factories: make(map[string]Factory),
	}
}

// Register adds a connector factory to the registry.
func (r *Registry) Register(connectorType string, factory Factory) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.factories[connectorType]; exists {
		return fmt.Errorf("connector type %q already registered", connectorType)
	}

	r.factories[connectorType] = factory
	return nil
}

// Create instantiates a new connector by type.
func (r *Registry) Create(connectorType string) (Connector, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	factory, exists := r.factories[connectorType]
	if !exists {
		return nil, fmt.Errorf("connector type %q not registered", connectorType)
	}

	return factory(), nil
}

// ListTypes returns all registered connector type names.
func (r *Registry) ListTypes() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	types := make([]string, 0, len(r.factories))
	for t := range r.factories {
		types = append(types, t)
	}
	return types
}

// Has returns whether a connector type is registered.
func (r *Registry) Has(connectorType string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	_, exists := r.factories[connectorType]
	return exists
}

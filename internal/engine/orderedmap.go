package engine

import "sync"

// OrderedMap holds the map and the order of keys
type OrderedMap struct {
	keys []string
	data map[string]interface{}
	mu   sync.Mutex
}

// NewOrderedMap creates a new OrderedMap
func NewOrderedMap() *OrderedMap {
	return &OrderedMap{
		keys: make([]string, 0),
		data: make(map[string]interface{}),
	}
}

// Set adds or updates a key-value pair in the OrderedMap
func (om *OrderedMap) Set(key string, value interface{}) {
	om.mu.Lock()
	defer om.mu.Unlock()
	if _, exists := om.data[key]; !exists {
		om.keys = append(om.keys, key)
	}
	om.data[key] = value
}

// Get retrieves a value by key from the OrderedMap
func (om *OrderedMap) Get(key string) (interface{}, bool) {
	om.mu.Lock()
	defer om.mu.Unlock()
	value, exists := om.data[key]
	return value, exists
}

// Keys returns the keys in the order they were added
func (om *OrderedMap) Keys() []string {
	om.mu.Lock()
	defer om.mu.Unlock()
	return om.keys
}

// Data returns the underlying map
func (om *OrderedMap) Data() map[string]interface{} {
	om.mu.Lock()
	defer om.mu.Unlock()
	return om.data
}

// Remove deletes a key-value pair from the OrderedMap
func (om *OrderedMap) Remove(key string) {
	om.mu.Lock()
	defer om.mu.Unlock()
	if _, exists := om.data[key]; exists {
		delete(om.data, key)
		for i, k := range om.keys {
			if k == key {
				om.keys = append(om.keys[:i], om.keys[i+1:]...)
				break
			}
		}
	}
}

// Clear resets the OrderedMap
func (om *OrderedMap) Clear() {
	om.mu.Lock()
	defer om.mu.Unlock()
	om.keys = make([]string, 0)
	om.data = make(map[string]interface{})
}

// IsEmpty checks if the OrderedMap is empty
func (om *OrderedMap) IsEmpty() bool {
	om.mu.Lock()
	defer om.mu.Unlock()
	return len(om.keys) == 0
}

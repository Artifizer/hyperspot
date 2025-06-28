package utils

/*
import (
	"weak"
)

// WeakCache provides a cache that holds weak references to values,
// allowing them to be garbage collected when no longer referenced elsewhere.
// T must be a pointer type (e.g., *Job)
type WeakCache[T any] struct {
	mu    DebugMutex
	stuff map[string]weak.Pointer[T]
}

func (c *WeakCache[T]) Lock() {
	c.mu.Lock()
}

func (c *WeakCache[T]) TryLock() bool {
	return c.mu.TryLock()
}

func (c *WeakCache[T]) Unlock() {
	c.mu.Unlock()
}

// Store adds a value to the cache with the given key.
// The value is stored as a weak reference.
// Note: T must be a pointer type (e.g., *Job)
func (c *WeakCache[T]) Store(key string, value T) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.stuff == nil {
		c.stuff = make(map[string]weak.Pointer[T])
	}

	// weak.Make requires a pointer, so we need to get the address of value
	// This assumes T is already a pointer type like *Job
	ptr := weak.Make[T](&value)
	c.stuff[key] = ptr
}

// Get retrieves a value from the cache by key.
// Returns the value and a boolean indicating if the key was found.
func (c *WeakCache[T]) Get(key string) (T, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	val, found := c.stuff[key]

	if !found {
		var zero T
		return zero, false // Nothing here!
	}

	if val.Value() == nil {
		delete(c.stuff, key)
		var zero T
		return zero, false
	}

	return *val.Value(), true
}

// Delete removes a key from the cache.
func (c *WeakCache[T]) Delete(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	delete(c.stuff, key)
}
*/

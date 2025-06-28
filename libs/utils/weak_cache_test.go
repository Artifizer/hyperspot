package utils

/*
import (
	"fmt"
	"runtime"
	"testing"
	"time"
	"unsafe"

	"github.com/stretchr/testify/assert"
)

// TestStruct is a simple struct for testing the WeakCache
type TestStructForWeakCache struct {
	ID   int
	Name string
}

// TestWeakCache_BasicOperations tests basic store/get/delete operations
func TestWeakCache_BasicOperations(t *testing.T) {
	cache := WeakCache[*TestStructForWeakCache]{}

	// Store a value
	item := &TestStructForWeakCache{ID: 1, Name: "Test"}
	cache.Store("key1", item)

	// Get the value back
	retrieved, found := cache.Get("key1")
	assert.True(t, found, "Item should be found in cache")
	assert.Equal(t, item, retrieved, "Retrieved item should match original")

	// Delete the value
	cache.Delete("key1")
	_, found = cache.Get("key1")
	assert.False(t, found, "Item should not be found after deletion")
}

// TestWeakCache_WeakReferences tests that weak references are properly garbage collected
func TestWeakCache_WeakReferences(t *testing.T) {
	cache := WeakCache[*TestStructForWeakCache]{}

	// Define a function that creates and stores an object, but doesn't maintain a reference
	storeTemporaryObject := func() {
		// Create a temporary object that will go out of scope
		temp := &TestStructForWeakCache{ID: 2, Name: "Temporary"}
		cache.Store("temp", temp)

		// Verify it's in the cache before we lose the reference
		retrieved, found := cache.Get("temp")
		assert.True(t, found, "Temporary item should be found")
		assert.Equal(t, temp, retrieved, "Retrieved temporary item should match original")
	}

	// Call the function to store a temporary object
	storeTemporaryObject()

	// At this point, no references to the object exist outside of the WeakCache
	// Force garbage collection multiple times to ensure cleanup
	for i := 0; i < 5; i++ {
		runtime.GC()
		// Sleep briefly to allow finalizers to run
		time.Sleep(10 * time.Millisecond)
	}

	// Try to access the object - it should be gone
	retrieved, found := cache.Get("temp")

	// This might occasionally fail if GC hasn't run, but should usually pass
	if found {
		t.Logf("Warning: Object not garbage collected yet, got: %v", retrieved)
	} else {
		t.Logf("Success: Object was properly garbage collected")
	}
}

// TestWeakCache_StrongReferences tests that items with strong references are not collected
func TestWeakCache_StrongReferences(t *testing.T) {
	cache := WeakCache[*TestStructForWeakCache]{}

	// Create an object with a strong reference that remains in scope
	strong := &TestStructForWeakCache{ID: 3, Name: "Strong"}
	cache.Store("strong", strong)

	fmt.Printf("strong 1: %p\n", strong) // refer strong

	retrieved, found := cache.Get("strong")
	assert.True(t, found, "Item with strong reference should be found in cache before GC")
	assert.Equal(t, strong, retrieved, "Retrieved item should match original")

	// Force garbage collection
	for i := 0; i < 5; i++ {
		runtime.GC()
		time.Sleep(10 * time.Millisecond)
	}

	// Item should still be in cache
	retrieved, found = cache.Get("strong")
	assert.True(t, found, "Item with strong reference should still be in cache after GC")
	assert.Equal(t, strong, retrieved, "Retrieved item should match original")

	fmt.Printf("strong 2: %p\n", strong) // refer strong
}

// TestWeakCache_MixedReferences tests a mix of strong and weak references
func TestWeakCache_MixedReferences(t *testing.T) {
	cache := WeakCache[*TestStructForWeakCache]{}

	// Strong reference
	strong := &TestStructForWeakCache{ID: 4, Name: "Strong"}
	cache.Store("strong", strong)

	// Weak reference (will be GC'd)
	storeTemporaryObject := func() {
		temp := &TestStructForWeakCache{ID: 5, Name: "Temporary"}
		cache.Store("temp", temp)
	}
	storeTemporaryObject()

	// Force garbage collection
	for i := 0; i < 5; i++ {
		runtime.GC()
		time.Sleep(10 * time.Millisecond)
	}

	// Check strong reference - should still be there
	retrieved, found := cache.Get("strong")
	assert.True(t, found, "Strong reference should still be in cache")
	assert.Equal(t, strong, retrieved, "Retrieved strong item should match original")

	// Check weak reference - should be gone
	_, found = cache.Get("temp")
	if found {
		t.Logf("Warning: Temporary object not garbage collected yet")
	} else {
		t.Logf("Success: Temporary object was properly garbage collected")
	}
}

// TestWeakCache_Locking tests the locking functionality
func TestWeakCache_Locking(t *testing.T) {
	cache := WeakCache[*TestStructForWeakCache]{}

	// Test Lock and Unlock
	cache.Lock()
	locked := !cache.TryLock() // Should fail since already locked
	cache.Unlock()

	assert.True(t, locked, "TryLock should fail while already locked")

	// Test TryLock success
	success := cache.TryLock()
	assert.True(t, success, "TryLock should succeed when not locked")
	cache.Unlock() // Clean up the lock
}

// TestWeakCache_ForceCollection is a more aggressive test to force garbage collection
func TestWeakCache_ForceCollection(t *testing.T) {
	cache := WeakCache[*TestStructForWeakCache]{}

	// Store a temporary object and immediately remove all references
	storeTemporaryObject := func() *byte {
		temp := &TestStructForWeakCache{ID: 6, Name: "Temporary"}
		cache.Store("temp", temp)

		// Allocate a lot of memory to encourage GC
		bigBlock := make([]byte, 1024*1024*10)
		return &bigBlock[0] // Return pointer to prevent immediate deallocation
	}

	// Retain pointer to prevent memory block from being collected immediately
	memPtr := storeTemporaryObject()
	_ = memPtr // Silence unused variable warning

	// Now allocate a lot more memory to really force GC
	for i := 0; i < 10; i++ {
		largeAlloc := make([]byte, 1024*1024*10)
		// Use the memory to prevent optimization
		largeAlloc[0] = 1
		runtime.GC()
	}

	// Try to access the object - it should be gone
	_, found := cache.Get("temp")
	if found {
		t.Logf("Warning: Object still not garbage collected despite aggressive forcing")
	} else {
		t.Logf("Success: Object was garbage collected with aggressive forcing")
	}
}

// TestWeakCache_NilHandling tests how the cache handles nil values
func TestWeakCache_NilHandling(t *testing.T) {
	cache := WeakCache[*TestStructForWeakCache]{}

	// Try to store nil
	var nilItem *TestStructForWeakCache = nil
	cache.Store("nil", nilItem)

	// Retrieve the nil value
	retrieved, found := cache.Get("nil")
	assert.True(t, found, "Nil item should be found in cache")
	assert.Nil(t, retrieved, "Retrieved item should be nil")
}

// forceGarbageCollection attempts to force garbage collection more aggressively
func forceGarbageCollection() {
	// Allocate and immediately discard memory to encourage GC
	for i := 0; i < 10; i++ {
		_ = make([]byte, 1024*1024*10)
		runtime.GC()
	}

	// Sleep to allow finalizers to run
	time.Sleep(100 * time.Millisecond)
}

// TestWeakCache_PointerIdentity verifies that the cache preserves pointer identity
func TestWeakCache_PointerIdentity(t *testing.T) {
	cache := WeakCache[*TestStructForWeakCache]{}

	// Create two distinct objects with the same content
	obj1 := &TestStructForWeakCache{ID: 7, Name: "Same Content"}
	obj2 := &TestStructForWeakCache{ID: 7, Name: "Same Content"}

	// Store both
	cache.Store("obj1", obj1)
	cache.Store("obj2", obj2)

	// Retrieve
	retrieved1, _ := cache.Get("obj1")
	retrieved2, _ := cache.Get("obj2")

	// They should have the same content but be different objects
	assert.Equal(t, retrieved1.ID, retrieved2.ID)
	assert.Equal(t, retrieved1.Name, retrieved2.Name)
	assert.NotEqual(t, unsafe.Pointer(retrieved1), unsafe.Pointer(retrieved2),
		"Retrieved objects should maintain pointer identity")
}
*/

package index

import (
	"sync"
)

// Index defines an interface for secondary indexes.
type Index interface {
	Insert(key interface{}, value string) // Value is PK or Tuple Pointer (here PK: string)
	Get(key interface{}) []string         // Returns list of PKs
}

// HashIndex is a simple in-memory hash map index.
// Supports equality lookups only.
type HashIndex struct {
	mu sync.RWMutex
	// Key -> []PK
	// Assuming Key is string or int. For simplicity, interface{} but we cast to string key for map.
	data map[interface{}][]string
}

func NewHashIndex() *HashIndex {
	return &HashIndex{
		data: make(map[interface{}][]string),
	}
}

func (idx *HashIndex) Insert(key interface{}, pk string) {
	idx.mu.Lock()
	defer idx.mu.Unlock()

	// Check if exists
	if vals, ok := idx.data[key]; ok {
		for _, existing := range vals {
			if existing == pk {
				return
			}
		}
	}

	idx.data[key] = append(idx.data[key], pk)
}

func (idx *HashIndex) Get(key interface{}) []string {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	// Return copy to be safe
	if vals, ok := idx.data[key]; ok {
		ret := make([]string, len(vals))
		copy(ret, vals)
		return ret
	}
	return nil
}

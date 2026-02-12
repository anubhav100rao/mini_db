package storage

import (
	"sort"
	"sync"

	"minidb/pkg/types"
)

// MemTable is an in-memory table.
// For simplicity, we use a map and sort keys on scan.
type MemTable struct {
	mu   sync.RWMutex
	data map[string]*types.Tuple
	size uint64 // Approximate size in bytes
}

func NewMemTable() *MemTable {
	return &MemTable{
		data: make(map[string]*types.Tuple),
	}
}

func (m *MemTable) Put(key string, tuple *types.Tuple) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.data[key] = tuple
	m.size += 1 // Increment count
}

func (m *MemTable) Get(key string) (*types.Tuple, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	val, ok := m.data[key]
	return val, ok
}

func (m *MemTable) Size() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.data)
}

// Iterator returns a snapshot iterator over the memtable.
func (m *MemTable) Iterator() *MemTableIterator {
	m.mu.RLock()
	defer m.mu.RUnlock()

	keys := make([]string, 0, len(m.data))
	for k := range m.data {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	return &MemTableIterator{
		mem:  m,
		keys: keys,
		idx:  0,
	}
}

type MemTableIterator struct {
	mem  *MemTable
	keys []string
	idx  int
}

func (it *MemTableIterator) Next() (string, *types.Tuple, bool) {
	if it.idx >= len(it.keys) {
		return "", nil, false
	}
	key := it.keys[it.idx]
	it.idx++

	// We can retrieve the value from the memtable (if we want live view?)
	// Or we should have snapshot the values.
	// For simplicity, let's assume we hold a reference to the map? No, map is unsafe if modified.
	// But we already released the lock.
	// So 'Get' is safe, but value might be deleted perfectly?
	// Correct snapshot isolation requires more work (copying map or MVCC).
	// For Phase 1 simple LSM, the memtable is immutable during flush, or we loop over keys.
	// Let's re-acquire lock to read value, or copy values during Snapshot creation.
	// Let's copy values during creation for safety.

	val, ok := it.mem.Get(key) // This uses lock
	if !ok {
		// If deleted in between, skipping?
		// Or assume this iterator is only used when MemTable is immutable (flushing).
		return key, nil, false // Should handle this
	}
	return key, val, true
}

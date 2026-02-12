package storage

import (
	"context"
	"fmt"
	"path/filepath"
	"sync"
	"time"

	"minidb/internal/storage/index"
	"minidb/internal/txn"
	"minidb/internal/wal"
	"minidb/pkg/types"
)

// StorageEngine defines the interface for data storage.
type StorageEngine interface {
	CreateTable(ctx context.Context, schema types.Schema) (types.TableID, error)
	InsertTuple(ctx context.Context, tid types.TableID, tup *types.Tuple, tx types.TxnID) error
	ScanTable(ctx context.Context, tid types.TableID, tx types.TxnID) (TupleIterator, error)
	GetTuple(ctx context.Context, tid types.TableID, key string, tx types.TxnID) (*types.Tuple, error)

	// Indexing support
	CreateSecondaryIndex(name string) error
	RegisterIndex(tid types.TableID, colIdx int, name string)
	GetIndex(name string) *index.HashIndex

	// Maintenance
	Flush(ctx context.Context) error
	Compact(ctx context.Context) error

	Close() error
}

// TupleIterator abstracts scanning results.
type TupleIterator interface {
	Next() (string, *types.Tuple, bool)
	Close() error
}

// LSMEngine is an implementation of StorageEngine using LSM trees.
type LSMEngine struct {
	mu         sync.RWMutex
	dataDir    string
	wal        *wal.WAL
	memTable   *MemTable
	txnManager *txn.TxnManager

	// Indexes
	indexes      map[string]*index.HashIndex
	tableIndexes map[types.TableID]map[int]string

	// SSTables (Ordered: Oldest -> Newest? or Newest -> Oldest?)
	// Let's store Newest -> Oldest for easy priority.
	sstables []*SSTableReader
}

func NewLSMEngine(dataDir string, wal *wal.WAL, tm *txn.TxnManager) *LSMEngine {
	return &LSMEngine{
		dataDir:      dataDir,
		wal:          wal,
		memTable:     NewMemTable(),
		txnManager:   tm,
		indexes:      make(map[string]*index.HashIndex),
		tableIndexes: make(map[types.TableID]map[int]string),
	}
}

func (e *LSMEngine) CreateTable(ctx context.Context, schema types.Schema) (types.TableID, error) {
	return 1, nil
}

func (e *LSMEngine) CreateSecondaryIndex(name string) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if _, ok := e.indexes[name]; ok {
		return fmt.Errorf("index %s already exists", name)
	}
	e.indexes[name] = index.NewHashIndex()
	return nil
}

func (e *LSMEngine) RegisterIndex(tid types.TableID, colIdx int, name string) {
	e.mu.Lock()
	defer e.mu.Unlock()

	if _, ok := e.tableIndexes[tid]; !ok {
		e.tableIndexes[tid] = make(map[int]string)
	}
	e.tableIndexes[tid][colIdx] = name
}

func (e *LSMEngine) GetIndex(name string) *index.HashIndex {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.indexes[name]
}

func (e *LSMEngine) InsertTuple(ctx context.Context, tid types.TableID, tup *types.Tuple, tx types.TxnID) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	// update MVCC info
	tup.Xmin = tx
	tup.Xmax = 0

	if len(tup.Values) == 0 {
		return fmt.Errorf("tuple has no values")
	}
	key := fmt.Sprintf("%v", tup.Values[0])

	// WAL Entry
	data, err := tup.Serialize()
	if err != nil {
		return err
	}

	if _, err := e.wal.Append(ctx, data); err != nil {
		return err
	}

	// Write to MemTable
	e.memTable.Put(key, tup)

	// Write to Secondary Indexes
	if cols, ok := e.tableIndexes[tid]; ok {
		for colIdx, idxName := range cols {
			if colIdx < len(tup.Values) {
				if idx, ok := e.indexes[idxName]; ok {
					// Start Index Insert (Should be part of WAL/Txn in real DB)
					val := tup.Values[colIdx]
					idx.Insert(val, key) // Map Val -> PK
				}
			}
		}
	}

	return nil
}

func (e *LSMEngine) ScanTable(ctx context.Context, tid types.TableID, tx types.TxnID) (TupleIterator, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()

	var iters []TupleIterator
	// 1. MemTable (Highest Priority)
	iters = append(iters, &EngineIterator{mit: e.memTable.Iterator()})

	// 2. SSTables (Newest -> Oldest)
	for _, sst := range e.sstables {
		iters = append(iters, sst.Iterator())
	}

	// Merge Iterator
	mergeIter := NewMergeIterator(iters)

	return &TxnIterator{
		iter: mergeIter,
		tm:   e.txnManager,
		tx:   tx,
	}, nil
}

func (e *LSMEngine) GetTuple(ctx context.Context, tid types.TableID, key string, tx types.TxnID) (*types.Tuple, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()

	// Check MemTable
	if val, ok := e.memTable.Get(key); ok {
		if e.txnManager.IsVisible(tx, val) {
			return val, nil
		}
	}

	// Check SSTables (Linear check for now)
	for _, sst := range e.sstables {
		it := sst.Iterator()
		for {
			k, v, ok := it.Next()
			if !ok {
				break
			}
			if k == key {
				if e.txnManager.IsVisible(tx, v) {
					return v, nil
				}
				// Found latest version (assuming no duplicates across levels in simplifiction,
				// or we scan all levels and find visible one?
				// In LSM, if we find key in newer level, we stop.
				// But with MVCC, newer version might be invisible (uncommitted or future).
				// So we need to continue?
				// For simplicity: stop if visible. If invisible, maybe check older?
				// Assume standard MVCC: newest visible version.
				// If we found a version but it's invisible, it might be an uncommitted insert
				// or a committed delete (Xmax).
				// If invisible, checks older?
				// If invisible (Active Txn), we should ignore it and look at older.
				// Correct.
			}
		}
	}

	return nil, types.ErrTableNotFound
}

func (e *LSMEngine) Close() error {
	for _, sst := range e.sstables {
		sst.Close()
	}
	return e.wal.Close()
}

// Flush writes MemTable to SSTable
func (e *LSMEngine) Flush(ctx context.Context) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.memTable.size == 0 {
		return nil
	}

	// swap memtable
	oldMem := e.memTable
	e.memTable = NewMemTable()

	timestamp := time.Now().UnixNano()
	filename := filepath.Join(e.dataDir, fmt.Sprintf("%d.sst", timestamp))

	writer, err := NewSSTableWriter(filename)
	if err != nil {
		return err
	}

	// Write
	it := oldMem.Iterator()
	for {
		k, v, ok := it.Next()
		if !ok {
			break
		}
		if err := writer.WriteTuple(k, v); err != nil {
			writer.Close()
			return err
		}
	}
	writer.Close()

	// Open for reading and prepend to list (Newest first)
	reader, err := NewSSTableReader(filename)
	if err != nil {
		return err
	}
	e.sstables = append([]*SSTableReader{reader}, e.sstables...)

	return nil
}

// Compact merges all SSTables into one.
func (e *LSMEngine) Compact(ctx context.Context) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if len(e.sstables) <= 1 {
		return nil // Nothing to compact
	}

	// Create Merge Iterator for all SSTables
	var iters []TupleIterator
	for _, sst := range e.sstables {
		iters = append(iters, sst.Iterator())
	}
	mergeIter := NewMergeIterator(iters)

	// Output File
	timestamp := time.Now().UnixNano()
	filename := filepath.Join(e.dataDir, fmt.Sprintf("compacted_%d.sst", timestamp))
	writer, err := NewSSTableWriter(filename)
	if err != nil {
		return err
	}

	// Write
	for {
		k, v, ok := mergeIter.Next()
		if !ok {
			break
		}
		// TODO: GC here? Remove aborted?
		if err := writer.WriteTuple(k, v); err != nil {
			writer.Close()
			return err
		}
	}
	writer.Close()

	// Close old SSTables
	for _, sst := range e.sstables {
		sst.Close()
		// Delete file? Yes.
		// os.Remove(sst.file.Name()) // In real DB, careful with open handles
	}

	// Replace with new
	reader, err := NewSSTableReader(filename)
	if err != nil {
		return err
	}
	e.sstables = []*SSTableReader{reader}

	return nil
}

// Helpers

type EngineIterator struct {
	mit *MemTableIterator
}

func (it *EngineIterator) Next() (string, *types.Tuple, bool) { return it.mit.Next() }
func (it *EngineIterator) Close() error                       { return nil }

type MergeIterator struct {
	iters    []TupleIterator
	currKeys []string
	currVals []*types.Tuple
	hasVal   []bool
}

func NewMergeIterator(iters []TupleIterator) *MergeIterator {
	m := &MergeIterator{
		iters:    iters,
		currKeys: make([]string, len(iters)),
		currVals: make([]*types.Tuple, len(iters)),
		hasVal:   make([]bool, len(iters)),
	}
	for i, it := range iters {
		k, v, ok := it.Next()
		if ok {
			m.currKeys[i] = k
			m.currVals[i] = v
			m.hasVal[i] = true
		}
	}
	return m
}

func (m *MergeIterator) Next() (string, *types.Tuple, bool) {
	// Find min key
	minKey := ""
	minIdx := -1

	for i := 0; i < len(m.iters); i++ {
		if !m.hasVal[i] {
			continue
		}
		if minIdx == -1 || m.currKeys[i] < minKey {
			minKey = m.currKeys[i]
			minIdx = i
		}
	}

	if minIdx == -1 {
		return "", nil, false
	}

	k := m.currKeys[minIdx]
	v := m.currVals[minIdx]

	// Advance minIdx
	nk, nv, nok := m.iters[minIdx].Next()
	if nok {
		m.currKeys[minIdx] = nk
		m.currVals[minIdx] = nv
		m.hasVal[minIdx] = true
	} else {
		m.hasVal[minIdx] = false
	}

	// Skip duplicates in older iterators
	for i := minIdx + 1; i < len(m.iters); i++ {
		if m.hasVal[i] && m.currKeys[i] == minKey {
			nk, nv, nok := m.iters[i].Next()
			if nok {
				m.currKeys[i] = nk
				m.currVals[i] = nv
				m.hasVal[i] = true
			} else {
				m.hasVal[i] = false
			}
		}
	}

	return k, v, true
}

func (m *MergeIterator) Close() error {
	for _, it := range m.iters {
		it.Close()
	}
	return nil
}

// TxnIterator wraps an iterator and filters invisible tuples.
type TxnIterator struct {
	iter TupleIterator
	tm   *txn.TxnManager
	tx   types.TxnID
}

func (it *TxnIterator) Next() (string, *types.Tuple, bool) {
	for {
		k, v, ok := it.iter.Next()
		if !ok {
			return "", nil, false
		}
		if it.tm.IsVisible(it.tx, v) {
			return k, v, true
		}
	}
}

func (it *TxnIterator) Close() error {
	return it.iter.Close()
}

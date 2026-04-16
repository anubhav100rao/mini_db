package storage

import (
	"context"
	"encoding/binary"
	"fmt"
	"os"
	"sort"
	"sync"
	"testing"

	"minidb/internal/txn"
	"minidb/internal/wal"
	"minidb/pkg/types"
)

// ------------------------------------------------------------
// Helpers
// ------------------------------------------------------------

func setupEngine(b *testing.B) (*LSMEngine, *txn.TxnManager, func()) {
	b.Helper()
	dir, err := os.MkdirTemp("", "minidb_bench")
	if err != nil {
		b.Fatal(err)
	}
	w, err := wal.NewWAL(dir + "/wal.log")
	if err != nil {
		b.Fatal(err)
	}
	tm := txn.NewTxnManager()
	engine := NewLSMEngine(dir, w, tm)
	return engine, tm, func() {
		engine.Close()
		os.RemoveAll(dir)
	}
}

func makeTuple(id int) *types.Tuple {
	return &types.Tuple{
		Values: []types.Datum{fmt.Sprintf("%08d", id), fmt.Sprintf("name_%d", id), id},
	}
}

// ------------------------------------------------------------
// Disk-backed B-Tree baseline
//
// A real B-Tree engine (Postgres, SQLite) does two things on
// every write:
//   1. Sequential WAL append  (same as LSM)
//   2. Random page write      (seek to sorted position, overwrite)
//
// Our previous baseline skipped both. This one does both,
// making the comparison fair.
// ------------------------------------------------------------

type diskBTree struct {
	mu      sync.Mutex
	keys    []string
	vals    [][]byte // serialised rows, simulating fixed-size pages
	walFile *os.File // WAL for durability — same as LSM
	pageFile *os.File // random-access page file simulating B-Tree pages
}

func newDiskBTree(dir string) (*diskBTree, error) {
	wf, err := os.OpenFile(dir+"/btree_wal.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return nil, err
	}
	pf, err := os.OpenFile(dir+"/btree_pages.db", os.O_CREATE|os.O_RDWR, 0644)
	if err != nil {
		wf.Close()
		return nil, err
	}
	return &diskBTree{walFile: wf, pageFile: pf}, nil
}

const pageSize = 4096 // 4 KB, typical B-Tree page

func (bt *diskBTree) insert(key string, row []byte) error {
	bt.mu.Lock()
	defer bt.mu.Unlock()

	// Step 1: WAL append (sequential) — same cost as LSM
	lenBuf := make([]byte, 4)
	binary.BigEndian.PutUint32(lenBuf, uint32(len(row)))
	if _, err := bt.walFile.Write(lenBuf); err != nil {
		return err
	}
	if _, err := bt.walFile.Write(row); err != nil {
		return err
	}

	// Step 2: Find sorted insertion point (B-Tree page navigation)
	i := sort.SearchStrings(bt.keys, key)
	bt.keys = append(bt.keys, "")
	bt.vals = append(bt.vals, nil)
	copy(bt.keys[i+1:], bt.keys[i:])
	copy(bt.vals[i+1:], bt.vals[i:])
	bt.keys[i] = key
	bt.vals[i] = row

	// Step 3: Random page write — seek to the page that owns this key
	// and overwrite it. This is the fundamental cost B-Trees pay that
	// LSM avoids: random I/O on the page file.
	pageIdx := int64(i / 10) // ~10 rows per page
	pageOffset := pageIdx * pageSize
	page := make([]byte, pageSize)
	copy(page, row)
	if _, err := bt.pageFile.WriteAt(page, pageOffset); err != nil {
		return err
	}

	return nil
}

func (bt *diskBTree) close() {
	bt.walFile.Close()
	bt.pageFile.Close()
}

// ------------------------------------------------------------
// Benchmarks
// ------------------------------------------------------------

// BenchmarkLSMInsert — full LSM write path: WAL (sequential) + MemTable.
func BenchmarkLSMInsert(b *testing.B) {
	engine, tm, cleanup := setupEngine(b)
	defer cleanup()
	ctx := context.Background()

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		tx := tm.Begin(ctx)
		if err := engine.InsertTuple(ctx, 1, makeTuple(i), tx.ID); err != nil {
			b.Fatal(err)
		}
		tm.Commit(ctx, tx)
	}

	b.ReportMetric(float64(b.N)/b.Elapsed().Seconds(), "TPS")
}

// BenchmarkDiskBTreeInsert — fair B-Tree baseline:
// WAL (sequential) + sorted in-memory structure + random page write on disk.
// This mirrors what Postgres/SQLite actually do on each INSERT.
func BenchmarkDiskBTreeInsert(b *testing.B) {
	dir, err := os.MkdirTemp("", "btree_bench")
	if err != nil {
		b.Fatal(err)
	}
	defer os.RemoveAll(dir)

	bt, err := newDiskBTree(dir)
	if err != nil {
		b.Fatal(err)
	}
	defer bt.close()

	payload := []byte(fmt.Sprintf("%s,%s,%d", "00000001", "name_bench", 42))

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		key := fmt.Sprintf("%08d", i)
		if err := bt.insert(key, payload); err != nil {
			b.Fatal(err)
		}
	}

	b.ReportMetric(float64(b.N)/b.Elapsed().Seconds(), "TPS")
}

// BenchmarkLSMMemTableOnly — isolated MemTable write speed with no WAL.
// Shows how fast the in-memory layer is on its own.
func BenchmarkLSMMemTableOnly(b *testing.B) {
	mem := NewMemTable()

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		mem.Put(fmt.Sprintf("%08d", i), makeTuple(i))
	}

	b.ReportMetric(float64(b.N)/b.Elapsed().Seconds(), "TPS")
}

// BenchmarkLSMConcurrentInsert — concurrent writers via MVCC.
// Each goroutine runs its own transaction; no writer blocks another.
func BenchmarkLSMConcurrentInsert(b *testing.B) {
	engine, tm, cleanup := setupEngine(b)
	defer cleanup()
	ctx := context.Background()

	b.ResetTimer()
	b.ReportAllocs()

	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			tx := tm.Begin(ctx)
			_ = engine.InsertTuple(ctx, 1, makeTuple(i), tx.ID)
			tm.Commit(ctx, tx)
			i++
		}
	})

	b.ReportMetric(float64(b.N)/b.Elapsed().Seconds(), "TPS")
}

// BenchmarkLSMPointLookup — single-key read from MemTable (O(1) hash lookup).
func BenchmarkLSMPointLookup(b *testing.B) {
	engine, tm, cleanup := setupEngine(b)
	defer cleanup()
	ctx := context.Background()

	const rows = 50_000
	for i := 0; i < rows; i++ {
		tx := tm.Begin(ctx)
		_ = engine.InsertTuple(ctx, 1, makeTuple(i), tx.ID)
		tm.Commit(ctx, tx)
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		key := fmt.Sprintf("%08d", i%rows)
		tx := tm.Begin(ctx)
		_, _ = engine.GetTuple(ctx, 1, key, tx.ID)
		tm.Commit(ctx, tx)
	}
}

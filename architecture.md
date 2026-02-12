# Introduction

We're building a **mini database engine** in Go: a single-process, embeddable DB that accepts SQL, plans and executes queries, manages transactions (MVCC), and persists data to disk using an LSM-style storage engine and a WAL. The goal is an educational, extendable codebase with clear package boundaries so you can incrementally implement and test components (parser → planner → executor → storage → recovery).

Below I convert your architecture into Go idioms: package layout, key interfaces and types, small example implementations/skeletons, and wiring notes so you can start coding right away.

---

# High-level architecture (Go package view)

```
cmd/minidb         # binary entrypoint (main)
internal/sql       # parser, AST, binder, logical plan, optimizer
internal/executor  # iterator model (operators), executor engine
internal/txn       # transaction manager (MVCC) + txn catalog
internal/storage   # storage engine (LSM), buffer pool
internal/wal       # write-ahead log + log replay
internal/catalog   # system tables, schemas, metadata
pkg/types          # tuples, datums, schema, ids, errors
```

Client → `internal/sql` (Parser → Analyzer/Binder → Optimizer) → `internal/executor` (physical operators) → `internal/txn` + `internal/storage` + `internal/wal` → disk

---

# Key Go interfaces & types (skeletons)

Below are compact, idiomatic Go interfaces and a few helper types to use as the contract between layers.

```go
// pkg/types/types.go
package types

import "context"

type TableID uint32
type TxnID uint64
type PageID uint64

type Datum interface{} // start simple; later use typed Datum

type Tuple struct {
    Values []Datum
}

type Schema struct {
    ColNames []string
    ColTypes []string
}

var ErrEOF = io.EOF
```

```go
// internal/executor/executor.go
package executor

import (
    "context"
    "io"
    "github.com/you/minidb/pkg/types"
)

// Executor is the iterator-style operator.
type Executor interface {
    Open(ctx context.Context) error
    Next(ctx context.Context) (*types.Tuple, error) // returns (nil, io.EOF) on end
    Close(ctx context.Context) error
}
```

Example SeqScan operator skeleton:

```go
// internal/executor/seqscan.go
package executor

import (
    "context"
    "io"
    "github.com/you/minidb/internal/storage"
    "github.com/you/minidb/pkg/types"
)

type SeqScan struct {
    tableID types.TableID
    store   storage.StorageEngine
    it      storage.TupleIterator
}

func NewSeqScan(tableID types.TableID, store storage.StorageEngine) *SeqScan {
    return &SeqScan{tableID: tableID, store: store}
}

func (s *SeqScan) Open(ctx context.Context) error {
    it, err := s.store.ScanTable(ctx, s.tableID)
    if err != nil { return err }
    s.it = it
    return nil
}

func (s *SeqScan) Next(ctx context.Context) (*types.Tuple, error) {
    if s.it == nil { return nil, io.EOF }
    t, err := s.it.Next(ctx)
    if err != nil { return nil, err }
    if t == nil { return nil, io.EOF }
    return t, nil
}

func (s *SeqScan) Close(ctx context.Context) error {
    if s.it != nil { s.it.Close() }
    s.it = nil
    return nil
}
```

```go
// internal/txn/txnmanager.go
package txn

import (
    "context"
    "sync/atomic"
    "github.com/you/minidb/pkg/types"
)

type TxnManager struct {
    nextTxID uint64 // atomic
    // snapshot and active txn bookkeeping would live here
}

func NewTxnManager() *TxnManager { return &TxnManager{nextTxID: 1} }

func (tm *TxnManager) Begin(ctx context.Context) types.TxnID {
    return types.TxnID(atomic.AddUint64(&tm.nextTxID, 1))
}

func (tm *TxnManager) Commit(ctx context.Context, tx types.TxnID) error {
    // write COMMIT to WAL, release resources
    return nil
}

func (tm *TxnManager) Abort(ctx context.Context, tx types.TxnID) error {
    // write ABORT, undo, release resources
    return nil
}

// MVCC visibility helper
func IsVisible(version *VersionedTuple, readTs types.TxnID) bool {
    if version == nil { return false }
    // xmin <= readTs and (xmax == 0 || xmax > readTs)
    if version.Xmin <= readTs && (version.Xmax == 0 || version.Xmax > readTs) {
        return true
    }
    return false
}

type VersionedTuple struct {
    Data *types.Tuple
    Xmin types.TxnID // creator
    Xmax types.TxnID // 0 if not deleted
}
```

```go
// internal/storage/storage.go
package storage

import (
    "context"
    "github.com/you/minidb/pkg/types"
)

// TupleIterator abstracts scanning results.
type TupleIterator interface {
    Next(ctx context.Context) (*types.Tuple, error)
    Close() error
}

type StorageEngine interface {
    CreateTable(ctx context.Context, schema types.Schema) (types.TableID, error)
    InsertTuple(ctx context.Context, tid types.TableID, tup *types.Tuple, tx types.TxnID) error
    ScanTable(ctx context.Context, tid types.TableID) (TupleIterator, error)
    GetTuple(ctx context.Context, tid types.TableID, key interface{}) (*types.Tuple, error)
    UpdateTuple(ctx context.Context, tid types.TableID, key interface{}, tup *types.Tuple, tx types.TxnID) error
    DeleteTuple(ctx context.Context, tid types.TableID, key interface{}, tx types.TxnID) error
    Flush(ctx context.Context) error
}
```

```go
// internal/wal/wal.go
package wal

import "context"

type WAL interface {
    Append(ctx context.Context, rec []byte) (uint64, error) // returns offset or lsn
    Flush(ctx context.Context) error
    Replay(ctx context.Context, handler func(rec []byte) error) error
    Close() error
}
```

---

# MVCC tuple & visibility (Go)

```go
type VersionedTuple struct {
    Data   *types.Tuple
    Xmin   types.TxnID // creator tx
    Xmax   types.TxnID // 0 = still visible
    Dirty  bool
}
```

Visibility check:

```go
func VisibleFor(tx types.TxnID, vt *VersionedTuple) bool {
    return vt != nil && vt.Xmin <= tx && (vt.Xmax == 0 || vt.Xmax > tx)
}
```

---

# Concurrency & Go-specific patterns

* Use `context.Context` for cancellation and deadlines in all APIs.
* Use `sync.RWMutex` for in-process metadata (catalog) reads/writes.
* Use `sync/atomic` for monotonic transaction ID generation.
* For background tasks (compaction, WAL flushing), use goroutines and channels:

  * `walAppendChan := make(chan []byte, 1024)` consumed by a single WAL writer goroutine (ensures serial WAL writes + batched fsync).
  * Compaction scheduler runs as a goroutine with a `time.Ticker`.
* For long-running operators (e.g., index backfill), support `ctx.Done()` checks in loops to stop early.

---

# Suggested package layout (filesystem)

```
minidb/
├─ cmd/minidb/main.go
├─ internal/
│  ├─ sql/          # parser, binder, logical/physical plans
│  ├─ executor/     # operators, materializers
│  ├─ txn/          # transaction manager, MVCC helpers
│  ├─ storage/      # LSM implementation + in-memory memtable
│  ├─ wal/          # WAL writer & replay
│  └─ catalog/      # system tables and metadata
└─ pkg/
   └─ types/
```

`main.go` wiring (very small sketch):

```go
func main() {
    ctx := context.Background()
    wal := wal.NewFileWAL("/var/tmp/minidb/wal")
    store := storage.NewLSMEngine(wal)
    tm := txn.NewTxnManager(wal, store)
    srv := sql.NewServer(store, tm) // exposes API or CLI
    log.Fatal(srv.Run(ctx))
}
```

---

# Implementation plan (phased milestones — no time estimates)

1. **Core storage & WAL**

   * Simple MemTable + immutable SSTable writer
   * WAL append + replay for recovery
   * Basic buffer pool / page cache (simple in-memory pages)

2. **Minimal SQL execution**

   * Integrate a SQL parser (use an existing parser like Vitess SQL parser or start with a tiny hand-written parser for `SELECT/INSERT`)
   * Build Binder -> LogicalPlan -> PhysicalPlan (very small)
   * Implement SeqScan, Filter, Projection operators

3. **Transactions & MVCC**

   * Implement TxnManager with atomic tx IDs
   * Store Xmin/Xmax on tuples, implement visibility checks
   * WAL records for BEGIN/COMMIT/ROW operations, crash recovery redo/undo

4. **Index & optimization**

   * Add simple secondary index (e.g., per-table hash index in memory)
   * Implement predicate pushdown and projection pushdown

5. **Polish**

   * Compaction, checkpointing, catalog persistence, tests & benchmarks

---

# Small notes, tradeoffs & recommendations

* **Start with LSM**: simpler for writes and for a single-process educational DB. Keep SSTable format simple (sorted binary key-values + sparse index).
* **MVCC** provides better read concurrency for learning; simpler to demo snapshot isolation.
* Use existing SQL parser if you want to focus on execution/storage (e.g., `vitess.io/vitess/go/vt/sqlparser`), otherwise implement a tiny subset parser first.
* Keep components behind small, well-documented interfaces so you can swap LSM ↔ B+Tree later.


# Storage Engine Internals

MiniDB uses an **LSM Tree (Log-Structured Merge Tree)** storage engine, optimized for write-heavy workloads while maintaining reasonable read performance through indexing and compaction.

## Core Components

### 1. Write-Ahead Log (WAL) (`internal/wal`)
- **Purpose**: Ensures durability (ACID).
- **Mechanism**: All modifications (`InsertTuple`) are first appended to an on-disk log file (`wal.log`) before being applied to memory.
- **Recovery**: On startup, the WAL is replayed to reconstruct the MemTable state, ensuring no data loss after a crash.

### 2. MemTable (`internal/storage/memtable.go`)
- **Structure**: In-memory sorted map (Go `map` with sorted keys during iteration).
- **Role**: Buffers recent writes. All reads check MemTable first.
- ** flushing**: When a manual `FLUSH` command is issued (or size limit reached in future logic), the MemTable is written to an SSTable.

### 3. SSTable (Sorted String Table) (`internal/storage/sstable.go`)
- **Structure**: Immutable on-disk file containing sorted Key-Value pairs.
- **Format**: Simple binary format:
    - `[KeyLen (4B)][Key (N bytes)][ValLen (4B)][Value (M bytes)]...`
- **Role**: Persistent storage.
- **Reading**: `SSTableIterator` scans the file sequentially.

## Operations

### Write Path
1.  **Append to WAL**: Write the tuple data to the log file.
2.  **Update MemTable**: Insert the tuple into the in-memory map.
3.  **Update Indexes**: If secondary indexes exist, update them (currently synchronous).

### Read Path (`GetTuple`, `ScanTable`)
1.  **Check MemTable**: Look for the key in memory. If found and visible (MVCC), return it.
2.  **Check SSTables**: Iterate through SSTables from newest to oldest.
    *   **Optimization**: A `MergeIterator` effectively merges the sorted streams from MemTable and all SSTables to provide a unified view.

### Compaction (`Compact` Method)
- **Goal**: Merge multiple small SSTables into a single large one to reduce read amplification and reclaim space from deleted/overwritten records.
- **Process**:
    1.  Create a `MergeIterator` over all existing SSTables.
    2.  Stream the merged, sorted unique keys (keeping the latest version) to a new SSTable file.
    3.  Close and delete old SSTable files.
    4.  Replace the engine's SSTable list with the single new file.

## Interface
The `StorageEngine` interface ensures abstraction:
```go
type StorageEngine interface {
    CreateTable(...)
    InsertTuple(...)
    ScanTable(...)
    GetTuple(...)
    CreateSecondaryIndex(...)
    Flush(...)
    Compact(...)
}
```

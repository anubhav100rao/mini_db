# Architecture Overview

MiniDB is a transactional, SQL-compliant, persistent database engine written in Go. It is designed for educational purposes, demonstrating core database concepts like LSM Trees, MVCC, WAL, and SQL execution.

## High-Level Components

The architecture follows a standard layered approach:

```mermaid
graph TD
    Client[Client / App] -->|SQL| Server[TCP Server / REPL]
    Server --> Parser[SQL Parser]
    Parser --> Binder[Binder / Analyzer]
    Binder --> Optimizer[Optimizer]
    Optimizer --> Executor[Execution Engine]
    Executor --> Transactions[Transaction Manager]
    Executor --> Storage[Storage Engine (LSM)]
    Storage --> WAL[Write-Ahead Log]
    Storage --> MemTable[MemTable (Memory)]
    Storage --> SSTable[SSTable (Disk)]
```

## Layer Descriptions

### 1. Interface Layer (`cmd/minidb`, `internal/server`)
- **REPL**: Interactive command-line interface for direct SQL execution.
- **TCP Server**: Listens on a port (default 3000) for newline-delimited SQL commands.

### 2. SQL Layer (`internal/sql`)
- **Parser**: Converts SQL strings into an Abstract Syntax Tree (AST). Supports `SELECT`, `INSERT`, and specific clauses like `WHERE`.
- **Binder**: Validates the AST against the catalog (schema) and produces a Logical Plan (e.g., `SeqScanNode`, `IndexScanNode`).
- **Optimizer**: Applies simple rules (like selecting an IndexScan over a SeqScan if a suitable index exists).

### 3. Execution Layer (`internal/executor`)
- Implements the **Volcano Iterator Model** (Open-Next-Close).
- Operators: `SeqScan`, `IndexScan`, `Filter`, `Insert`, `Project`.
- Executors requesting tuples from the Storage Engine.

### 4. Transaction Layer (`internal/txn`)
- Manages transaction lifecycles (`Begin`, `Commit`, `Abort`).
- Assigns monotonic Transaction IDs.
- Implements **MVCC (Multi-Version Concurrency Control)** visibility rules to support **Snapshot Isolation**.

### 5. Storage Layer (`internal/storage`)
- **LSM Tree (Log-Structured Merge Tree)**:
    - **MemTable**: In-memory mutable structure (sorted map) for recent writes.
    - **SSTable**: Immutable on-disk files for older data.
    - **WAL**: Append-only log for durability and crash recovery.
- **Compaction**: Merges multiple SSTables into a single file to reclaim space and improve read performance.

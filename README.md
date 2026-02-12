# MiniDB - The Go Database Engine

![Status](https://img.shields.io/badge/status-active-success.svg)
![License](https://img.shields.io/badge/license-MIT-blue.svg)

**MiniDB** is a lightweight, transactional, SQL-compliant database engine written in Go. Based on the **LSM Tree (Log-Structured Merge Tree)** architecture, it is designed for educational purposes to demonstrate core database internals including MVCC, WAL, compaction, and query execution.

## 🚀 Key Features

*   **Storage Engine**: LSM Tree (MemTable + SSTables) optimized for write-heavy workloads.
*   **Durability**: Write-Ahead Logging (WAL) ensures data is never lost, even after a crash.
*   **Transactions**: Full ACID support with **Snapshot Isolation** using Multi-Version Concurrency Control (MVCC).
*   **SQL Support**: Custom parser and execution engine supporting `SELECT`, `INSERT`, and `WHERE` clauses.
*   **Indexing**: Hash Index support for O(1) primary key lookups.
*   **Compaction**: Background merging of SSTables to reclaim space and improve read performance.
*   **Interfaces**: Interactive CLI (REPL) and TCP Server mode.

## 📦 Getting Started

### Prerequisites
*   Go 1.20 or higher

### Installation

Clone the repository and build using Make:

```bash
git clone https://github.com/yourusername/minidb.git
cd minidb
make build
```

### Running the Database

**1. Interactive Mode (REPL)**
```bash
make run
```
Or manually: `./minidb`

**2. Server Mode**
```bash
make server
```
Or manually: `./minidb -server -port 3000`

### Testing
Run all tests:
```bash
make test
```

## 📚 Documentation

Detailed documentation is available in the `docs/` directory:

- [Architecture Overview](docs/architecture.md)
- [Storage Engine Internals](docs/storage.md)
- [SQL Reference](docs/sql.md)
- [Transaction & MVCC Model](docs/transactions.md)
- [User Guide](docs/usage.md)

## 🧪 Testing

Run the comprehensive test suite to ensure everything is working:

```bash
go test ./...
```

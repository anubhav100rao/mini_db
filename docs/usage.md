# User Guide

## Building
Requires Go 1.20+.

```bash
cd minidb
go build -o minidb ./cmd/minidb
```

## Running Verification Tests
To ensure everything is working correctly:
```bash
go test ./...
```

## Interactive REPL
Start the database in interactive mode:
```bash
./minidb
```
You will see the prompt `minidb>`.

**Example Session:**
```sql
minidb> INSERT INTO users VALUES (1, 'Alice')
minidb> INSERT INTO users VALUES (2, 'Bob')
minidb> SELECT * FROM users WHERE id = 1
[1 Alice]
(1 rows)
minidb> FLUSH
MemTable flushed to SSTable
minidb> exit
```

## TCP Server Mode
Start the database as a server:
```bash
./minidb -server -port 3000
```

Connect using a TCP client like `nc`:
```bash
nc localhost 3000
INSERT INTO users VALUES (10, 'Remote')
SELECT * FROM users
```

## Data Persistence
Data is stored in the `./data` directory within the working folder.
- `wal.log`: Write-Ahead Log.
- `*.sst`: Compacted data files.

To reset the database, simply delete the `./data` directory.

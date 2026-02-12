# SQL Reference & Execution

MiniDB supports a subset of SQL for data manipulation and definition.

## Supported Syntax

### Data Definition
MiniDB currently creates schemas programmatically in `main.go` for the `users` table. Dynamic `CREATE TABLE` via SQL is not yet exposed in the parser.
Schema for `users`: `id (int), name (string)`.

### Data Manipulation

**INSERT**
```sql
INSERT INTO users VALUES (1, 'alice')
INSERT INTO users VALUES (2, 'bob'), (3, 'charlie')
```

**SELECT**
```sql
SELECT * FROM users
SELECT * FROM users WHERE id = 1
SELECT * FROM users WHERE name = 'alice'
```
*   `SELECT *` is the only projected column set supported.
*   `WHERE` supports simple equality checks (`=`).

### Maintenance Commands
These are custom extensions to manage the storage engine.
```sql
FLUSH    -- Flushes MemTable to Disk (SSTable)
COMPACT  -- Merges all SSTables
```

### Transaction Control
```sql
BEGIN
COMMIT
ROLLBACK (or ABORT)
```

## Execution Flow

1.  **Parser**: The custom recursive-descent parser (`internal/sql/parser`) reads the SQL string.
    *   If valid, returns a `Statement` node (e.g., `SelectStmt`).
    *   If invalid, returns a syntax error.

2.  **Binder**: The binder (`internal/sql/binder`) resolves table and column names against the Catalog.
    *   It converts `Statement` (AST) -> `LogicalPlan`.
    *   **Optimization**: If a `WHERE` clause references an indexed column (e.g., `id`), the Binder generates an `IndexScanNode` instead of a `SeqScanNode` + `FilterNode`.

3.  **Executor**: The logic plan is converted into a tree of physical **Iterators**.
    *   `Next()` is called repeatedly on the root iterator.
    *   Each iterator pulls data from its child, processes it (filters, projects), and returns it up the stack.

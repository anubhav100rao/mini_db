# Transaction & MVCC Model

MiniDB implements **Snapshot Isolation** using **Multi-Version Concurrency Control (MVCC)**.

## Transaction Components

- **TxnID**: A monotonically increasing 64-bit integer assigned to every transaction.
- **TxnManager**: Tracks the state of all transactions (`Active`, `Committed`, `Aborted`).

## MVCC Implementation

Every tuple in the database is versioned with two metadata fields:
- **Xmin**: The ID of the transaction that **created** (inserted/updated) this tuple version.
- **Xmax**: The ID of the transaction that **deleted** (or updated) this tuple version. (0 if not deleted).

### Visibility Rules
When a transaction `T` reads a tuple, the `TxnManager.IsVisible(T, Tuple)` function determines if it can be seen:

1.  **Created by T**: If `Xmin == T` (and not deleted by T), it is visible.
2.  **Created by Active**: If `Xmin` is active (uncommitted) and `!= T`, it is **invisible**.
3.  **Created by Committed**: If `Xmin` is committed:
    *   Check `Xmax` (Deletion):
        *   If `Xmax == 0`, it is **visible**.
        *   If `Xmax` is active (uncommitted) and `!= T`, it is **visible** (delete hasn't happened yet for us).
        *   If `Xmax` is committed, it is **invisible** (it was deleted).

This ensures readers don't block writers and vice versa, providing a consistent view of the database as of the start of the transaction.

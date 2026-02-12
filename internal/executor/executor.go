package executor

import (
	"context"
	"minidb/internal/storage"
	"minidb/pkg/types"
)

// Executor represents an execution operator.
type Executor interface {
	Open(ctx context.Context) error
	Next(ctx context.Context) (*types.Tuple, error)
	Close(ctx context.Context) error
}

// Global Execution Context (e.g., Transaction ID)
type ExecContext struct {
	Storage storage.StorageEngine
	TxnID   types.TxnID
}

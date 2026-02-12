package executor

import (
	"context"
	"io"
	"minidb/internal/sql/plan"
	"minidb/internal/storage/index"
	"minidb/pkg/types"
)

type IndexScan struct {
	ctx    *ExecContext
	plan   *plan.IndexScanNode
	idx    *index.HashIndex
	keys   []string // List of PKs to fetch
	cursor int
}

func NewIndexScan(ctx *ExecContext, node *plan.IndexScanNode, idx *index.HashIndex) *IndexScan {
	return &IndexScan{ctx: ctx, plan: node, idx: idx}
}

func (s *IndexScan) Open(ctx context.Context) error {
	// Lookup keys in index
	val := s.plan.Value // The value we are looking for (WHERE col = val)

	// We assume Index stores PKs as string
	s.keys = s.idx.Get(val)
	s.cursor = 0
	return nil
}

func (s *IndexScan) Next(ctx context.Context) (*types.Tuple, error) {
	if s.cursor >= len(s.keys) {
		return nil, io.EOF
	}

	// Fetch tuple by PK
	pk := s.keys[s.cursor]
	s.cursor++

	tuple, err := s.ctx.Storage.GetTuple(ctx, s.plan.TableID, pk, s.ctx.TxnID)
	if err != nil {
		// If not found (cleaned up?) skip?
		// Or return error?
		// GetTuple returns ErrTableNotFound if not found.
		// But index might be stale.
		// Let's treat as skip if not found?
		// For now, return error or handle gracefully.
		return nil, err
	}

	return tuple, nil
}

func (s *IndexScan) Close(ctx context.Context) error {
	return nil
}

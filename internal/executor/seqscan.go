package executor

import (
	"context"
	"io"
	"minidb/internal/sql/plan"
	"minidb/internal/storage"
	"minidb/pkg/types"
)

type SeqScan struct {
	ctx  *ExecContext
	plan *plan.SeqScanNode
	iter storage.TupleIterator
}

func NewSeqScan(ctx *ExecContext, node *plan.SeqScanNode) *SeqScan {
	return &SeqScan{ctx: ctx, plan: node}
}

func (s *SeqScan) Open(ctx context.Context) error {
	// Pass s.ctx.TxnID to ScanTable
	it, err := s.ctx.Storage.ScanTable(ctx, s.plan.TableID, s.ctx.TxnID)
	if err != nil {
		return err
	}
	s.iter = it
	return nil
}

func (s *SeqScan) Next(ctx context.Context) (*types.Tuple, error) {
	if s.iter == nil {
		return nil, io.EOF
	}

	// Iterate until we find a valid tuple (visible).
	// The storage engine now handles visibility checks via TxnIterator.
	// So we just return the next tuple.
	_, tuple, ok := s.iter.Next()
	if !ok {
		return nil, io.EOF
	}
	return tuple, nil
}

func (s *SeqScan) Close(ctx context.Context) error {
	if s.iter != nil {
		return s.iter.Close()
	}
	return nil
}

package executor

import (
	"context"
	"io"
	"minidb/internal/sql/plan"
	"minidb/pkg/types"
)

type InsertExec struct {
	ctx    *ExecContext
	plan   *plan.InsertNode
	cursor int
}

func NewInsertExec(ctx *ExecContext, node *plan.InsertNode) *InsertExec {
	return &InsertExec{ctx: ctx, plan: node}
}

func (e *InsertExec) Open(ctx context.Context) error {
	e.cursor = 0
	return nil
}

func (e *InsertExec) Next(ctx context.Context) (*types.Tuple, error) {
	if e.cursor >= len(e.plan.Values) {
		return nil, io.EOF
	}

	// Insert the row into storage
	values := e.plan.Values[e.cursor]
	tuple := &types.Tuple{Values: values}

	// InsertTuple needs TableID and TxnID
	if err := e.ctx.Storage.InsertTuple(ctx, e.plan.TableID, tuple, e.ctx.TxnID); err != nil {
		return nil, err
	}

	e.cursor++

	// Return the inserted tuple as "result" (like RETURNING *)
	// Or return nil if we emulate Standard SQL (where INSERT returns count, usually handled by shell)
	// For simplicity, let's return the tuple to show it was inserted.
	return tuple, nil
}

func (e *InsertExec) Close(ctx context.Context) error {
	return nil
}

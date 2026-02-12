package executor

import (
	"context"
	"fmt"
	"io"
	"minidb/internal/sql/plan"
	"minidb/pkg/types"
)

type FilterExec struct {
	ctx   *ExecContext
	plan  *plan.FilterNode
	child Executor
}

func NewFilterExec(ctx *ExecContext, node *plan.FilterNode, child Executor) *FilterExec {
	return &FilterExec{ctx: ctx, plan: node, child: child}
}

func (e *FilterExec) Open(ctx context.Context) error {
	return e.child.Open(ctx)
}

func (e *FilterExec) Next(ctx context.Context) (*types.Tuple, error) {
	for {
		t, err := e.child.Next(ctx)
		if err != nil {
			return nil, err
		}
		if t == nil {
			return nil, io.EOF
		}

		// Evaluate Filter (Only = supported for now)
		val := t.Values[e.plan.ColIdx]

		match := false
		// Type agnostic comparison (simplified)
		switch v1 := val.(type) {
		case int:
			if v2, ok := e.plan.Value.(int); ok {
				match = (v1 == v2)
			}
		case string:
			if v2, ok := e.plan.Value.(string); ok {
				match = (v1 == v2)
			}
		default:
			return nil, fmt.Errorf("unsupported type for comparison")
		}

		if match {
			return t, nil
		}
		// Loop again
	}
}

func (e *FilterExec) Close(ctx context.Context) error {
	return e.child.Close(ctx)
}

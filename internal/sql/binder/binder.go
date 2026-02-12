package binder

import (
	"fmt"

	"minidb/internal/catalog"
	"minidb/internal/sql/parser"
	"minidb/internal/sql/plan"
	"minidb/pkg/types"
)

type Binder struct {
	catalog *catalog.Catalog
}

func NewBinder(c *catalog.Catalog) *Binder {
	return &Binder{catalog: c}
}

func (b *Binder) Bind(stmt parser.Statement) (plan.LogicalPlan, error) {
	switch s := stmt.(type) {
	case *parser.SelectStmt:
		return b.bindSelect(s)
	case *parser.InsertStmt:
		return b.bindInsert(s)
	default:
		return nil, fmt.Errorf("unsupported statement type")
	}
}

func (b *Binder) bindSelect(stmt *parser.SelectStmt) (plan.LogicalPlan, error) {
	tableInfo, err := b.catalog.GetTableByName(stmt.TableName)
	if err != nil {
		return nil, err
	}

	// Create Scan Node
	var root plan.LogicalPlan = &plan.SeqScanNode{
		TableID:     tableInfo.ID,
		TableSchema: tableInfo.Schema,
	}

	// Bind WHERE
	if stmt.Where != nil {
		// Resolve Column name to Index
		colIdx := -1
		for i, name := range tableInfo.Schema.ColNames {
			if name == stmt.Where.Column {
				colIdx = i
				break
			}
		}
		if colIdx == -1 {
			return nil, fmt.Errorf("column %s not found", stmt.Where.Column)
		}

		// Optimizer: Check if Index exists for this column
		if idxName, ok := tableInfo.Indexes[stmt.Where.Column]; ok {
			// Use IndexScan!
			root = &plan.IndexScanNode{
				TableID:     tableInfo.ID,
				TableSchema: tableInfo.Schema,
				IndexName:   idxName,
				Value:       stmt.Where.Value,
			}
		} else {
			// Fallback to SeqScan + Filter
			root = &plan.FilterNode{
				Child:  root,
				Column: stmt.Where.Column,
				Op:     stmt.Where.Op,
				Value:  stmt.Where.Value,
				ColIdx: colIdx,
			}
		}
	}

	return root, nil
}

func (b *Binder) bindInsert(stmt *parser.InsertStmt) (plan.LogicalPlan, error) {
	tableInfo, err := b.catalog.GetTableByName(stmt.TableName)
	if err != nil {
		return nil, err
	}

	var data [][]types.Datum
	for _, row := range stmt.Values {
		var tupleValues []types.Datum
		for _, val := range row {
			tupleValues = append(tupleValues, val)
		}
		data = append(data, tupleValues)
	}

	return &plan.InsertNode{
		TableID: tableInfo.ID,
		Values:  data,
	}, nil
}

package plan

import (
	"minidb/pkg/types"
)

type PlanType int

const (
	PlanSeqScan PlanType = iota
	PlanInsert
	PlanProject
	PlanFilter
	PlanIndexScan
)

type LogicalPlan interface {
	Type() PlanType
	Schema() types.Schema
	Children() []LogicalPlan
}

// SeqScanNode represents a full table scan.
type SeqScanNode struct {
	TableID     types.TableID
	TableSchema types.Schema
}

func (n *SeqScanNode) Type() PlanType          { return PlanSeqScan }
func (n *SeqScanNode) Schema() types.Schema    { return n.TableSchema }
func (n *SeqScanNode) Children() []LogicalPlan { return nil }

// InsertNode represents an insertion.
type InsertNode struct {
	TableID types.TableID
	Values  [][]types.Datum // List of rows to insert
}

func (n *InsertNode) Type() PlanType          { return PlanInsert }
func (n *InsertNode) Schema() types.Schema    { return types.Schema{} }
func (n *InsertNode) Children() []LogicalPlan { return nil }

// FilterNode filters rows from a child node.
type FilterNode struct {
	Child  LogicalPlan
	Column string
	Op     string
	Value  interface{}
	// Metadata: Column Index to easy execution
	ColIdx int
}

func (n *FilterNode) Type() PlanType          { return PlanFilter }
func (n *FilterNode) Schema() types.Schema    { return n.Child.Schema() }
func (n *FilterNode) Children() []LogicalPlan { return []LogicalPlan{n.Child} }

// IndexScanNode represents an index lookup.
type IndexScanNode struct {
	TableID     types.TableID
	TableSchema types.Schema
	IndexName   string
	Value       interface{} // Look for equality
}

func (n *IndexScanNode) Type() PlanType          { return PlanIndexScan }
func (n *IndexScanNode) Schema() types.Schema    { return n.TableSchema }
func (n *IndexScanNode) Children() []LogicalPlan { return nil }

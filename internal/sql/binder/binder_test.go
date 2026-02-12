package binder

import (
	"minidb/internal/catalog"
	"minidb/internal/sql/parser"
	"minidb/internal/sql/plan"
	"minidb/pkg/types"
	"testing"
)

func TestBindSelect(t *testing.T) {
	cat := catalog.NewCatalog()
	schema := types.Schema{
		ColNames: []string{"id", "name"},
		ColTypes: []string{"int", "string"},
	}
	cat.CreateTable("users", schema)

	b := NewBinder(cat)
	stmt := &parser.SelectStmt{
		TableName: "users",
		Where: &parser.WhereClause{
			Column: "id",
			Op:     "=",
			Value:  123,
		},
	}

	p, err := b.Bind(stmt)
	if err != nil {
		t.Fatalf("Bind failed: %v", err)
	}

	// Should be Filter -> SeqScan (no index yet)
	filter, ok := p.(*plan.FilterNode)
	if !ok {
		t.Fatalf("Expected FilterNode, got %T", p)
	}

	if filter.Column != "id" {
		t.Errorf("Expected filter on id, got %s", filter.Column)
	}

	_, ok = filter.Child.(*plan.SeqScanNode)
	if !ok {
		t.Errorf("Expected SeqScan child, got %T", filter.Child)
	}
}

func TestBindIndexSelection(t *testing.T) {
	cat := catalog.NewCatalog()
	schema := types.Schema{
		ColNames: []string{"id", "name"},
		ColTypes: []string{"int", "string"},
	}
	cat.CreateTable("users", schema)
	cat.CreateIndex("users", "id", "idx_id")

	b := NewBinder(cat)
	stmt := &parser.SelectStmt{
		TableName: "users",
		Where: &parser.WhereClause{
			Column: "id", // Indexed column
			Op:     "=",
			Value:  123,
		},
	}

	p, err := b.Bind(stmt)
	if err != nil {
		t.Fatalf("Bind failed: %v", err)
	}

	// Should be IndexScan
	idxScan, ok := p.(*plan.IndexScanNode)
	if !ok {
		t.Fatalf("Expected IndexScanNode, got %T", p)
	}

	if idxScan.IndexName != "idx_id" {
		t.Errorf("Expected index idx_id, got %s", idxScan.IndexName)
	}
}

func TestBindTableNotFound(t *testing.T) {
	cat := catalog.NewCatalog()
	b := NewBinder(cat)
	stmt := &parser.SelectStmt{TableName: "missing_table"}

	if _, err := b.Bind(stmt); err == nil {
		t.Error("Expected error for missing table")
	}
}

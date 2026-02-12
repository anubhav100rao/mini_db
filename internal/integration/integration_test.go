package integration

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"

	"minidb/internal/catalog"
	"minidb/internal/executor"
	"minidb/internal/sql/binder"
	"minidb/internal/sql/parser"
	"minidb/internal/sql/plan"
	"minidb/internal/storage"
	"minidb/internal/txn"
	"minidb/internal/wal"
	"minidb/pkg/types"
)

func TestEndToEndSQL(t *testing.T) {
	// Setup
	tmpDir, _ := os.MkdirTemp("", "minidb_integration")
	defer os.RemoveAll(tmpDir)

	walLog, _ := wal.NewWAL(tmpDir + "/wal.log")
	tm := txn.NewTxnManager()
	engine := storage.NewLSMEngine(tmpDir, walLog, tm)
	defer engine.Close()

	cat := catalog.NewCatalog()
	schema := types.Schema{
		ColNames: []string{"id", "name"},
		ColTypes: []string{"int", "string"},
	}
	tid, _ := cat.CreateTable("users", schema)

	// Create Index
	engine.CreateSecondaryIndex("idx_id")
	engine.RegisterIndex(tid, 0, "idx_id")
	cat.CreateIndex("users", "id", "idx_id")

	// Helper to execute SQL
	execSQL := func(sql string) []string {
		// Just like main.go's ExecuteQuery logic but returning rows
		stmt, err := parser.Parse(sql)
		if err != nil {
			t.Fatalf("Parse error: %v", err)
		}
		b := binder.NewBinder(cat)
		p, err := b.Bind(stmt)
		if err != nil {
			t.Fatalf("Bind error: %v", err)
		}

		tx := tm.Begin(context.Background())
		defer tm.Commit(context.Background(), tx)

		ctx := &executor.ExecContext{Storage: engine, TxnID: tx.ID}
		var exec executor.Executor

		switch node := p.(type) {
		case *plan.SeqScanNode:
			exec = executor.NewSeqScan(ctx, node)
		case *plan.InsertNode:
			exec = executor.NewInsertExec(ctx, node)
		case *plan.FilterNode:
			childExec := executor.NewSeqScan(ctx, node.Child.(*plan.SeqScanNode))
			exec = executor.NewFilterExec(ctx, node, childExec)
		case *plan.IndexScanNode:
			idx := engine.GetIndex(node.IndexName)
			exec = executor.NewIndexScan(ctx, node, idx)
		}

		if err := exec.Open(context.Background()); err != nil {
			t.Fatalf("Executor Open error: %v", err)
		}
		defer exec.Close(context.Background())

		var results []string
		for {
			tuple, err := exec.Next(context.Background())
			if tuple == nil {
				break
			}
			if err != nil {
				break
			}
			results = append(results, fmt.Sprintf("%v", tuple.Values))
		}
		return results
	}

	// 1. Insert
	execSQL("INSERT INTO users VALUES (1, 'alice')")
	execSQL("INSERT INTO users VALUES (2, 'bob')")

	// 2. Select All
	rows := execSQL("SELECT * FROM users")
	if len(rows) != 2 {
		t.Errorf("Expected 2 rows, got %d", len(rows))
	}

	// 3. Select Where (Index Scan)
	rows = execSQL("SELECT * FROM users WHERE id = 1")
	// NOTE: In real test we can't easily verify checks "Explain: IndexScan" output here unless we capture stdout or inspect logic.
	// But we check correctness.
	if len(rows) != 1 || !strings.Contains(rows[0], "alice") {
		t.Errorf("Index lookup failed. Got: %v", rows)
	}

	// 4. Select Where (Seq Scan / Filter)
	rows = execSQL("SELECT * FROM users WHERE name = 'bob'")
	if len(rows) != 1 || !strings.Contains(rows[0], "bob") {
		t.Errorf("Filter lookup failed. Got: %v", rows)
	}

	// 5. Compaction Test
	engine.Flush(context.Background())                                                                                                  // Flush MemTable -> SSTable
	engine.InsertTuple(context.Background(), tid, &types.Tuple{Values: []types.Datum{3, "charlie"}}, tm.Begin(context.Background()).ID) // Add more data to MemTable
	engine.Flush(context.Background())                                                                                                  // Flush again -> 2 SSTables

	// Verify before compact
	rows = execSQL("SELECT * FROM users")
	if len(rows) != 3 {
		t.Errorf("Expected 3 rows (alice, bob, charlie), got %d", len(rows))
	}

	// Compact
	if err := engine.Compact(context.Background()); err != nil {
		t.Fatalf("Compact failed: %v", err)
	}

	// Verify after compact
	rows = execSQL("SELECT * FROM users")
	if len(rows) != 3 {
		t.Errorf("Expected 3 rows after compaction, got %d", len(rows))
	}
}

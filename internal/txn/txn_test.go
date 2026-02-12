package txn

import (
	"context"
	"minidb/pkg/types"
	"testing"
)

func TestTxnManager(t *testing.T) {
	tm := NewTxnManager()

	// Test Begin
	ctx := context.Background()
	// First Txn might be 2 (nextTxID=1 + 1)
	tx1 := tm.Begin(ctx)
	if tx1.State != TxnActive {
		t.Errorf("Expected Active state")
	}

	tx2 := tm.Begin(ctx)
	if tx2.ID == tx1.ID {
		t.Errorf("Txn IDs should be distinct")
	}

	// Test Commit
	if err := tm.Commit(ctx, tx1); err != nil {
		t.Errorf("Commit failed: %v", err)
	}
	if tm.GetTxnState(tx1.ID) != TxnCommitted {
		t.Errorf("Expected Txn1 to be Committed")
	}

	// Test Abort
	if err := tm.Abort(ctx, tx2); err != nil {
		t.Errorf("Abort failed: %v", err)
	}
	if tm.GetTxnState(tx2.ID) != TxnAborted {
		t.Errorf("Expected Txn2 to be Aborted")
	}
}

func TestVisibility(t *testing.T) {
	tm := NewTxnManager()
	ctx := context.Background()

	t1 := tm.Begin(ctx) // Creator
	t2 := tm.Begin(ctx) // Reader
	t3 := tm.Begin(ctx) // Deleter

	// Case 1: Created by current txn -> Visible
	tuple := &types.Tuple{Xmin: t1.ID, Xmax: 0}
	if !tm.IsVisible(t1.ID, tuple) {
		t.Error("Tuple created by current txn should be visible")
	}

	// Case 2: Created by active uncommitted txn -> Invisible to others
	if tm.IsVisible(t2.ID, tuple) {
		t.Error("Tuple created by active uncommitted txn should be invisible to others")
	}

	// Case 3: Created by committed txn -> Visible
	tm.Commit(ctx, t1)
	if !tm.IsVisible(t2.ID, tuple) {
		t.Error("Tuple created by committed txn should be visible")
	}

	// Case 4: Deleted by current txn -> Invisible
	tuple.Xmax = t2.ID
	if tm.IsVisible(t2.ID, tuple) {
		t.Error("Tuple deleted by current txn should be invisible")
	}

	// Case 5: Deleted by other active txn -> Visible (Snapshot Isolation / Read Committed)
	tuple.Xmax = t3.ID
	if !tm.IsVisible(t2.ID, tuple) {
		t.Error("Tuple deleted by other active txn should be visible")
	}

	// Case 6: Deleted by committed txn -> Invisible
	tm.Commit(ctx, t3)
	if tm.IsVisible(t2.ID, tuple) {
		t.Error("Tuple deleted by committed txn should be invisible")
	}
}

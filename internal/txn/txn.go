package txn

import (
	"context"
	"minidb/pkg/types"
	"sync"
	"sync/atomic"
)

type TxnState int

const (
	TxnActive TxnState = iota
	TxnCommitted
	TxnAborted
)

type Transaction struct {
	ID    types.TxnID
	State TxnState
}

type TxnManager struct {
	mu         sync.RWMutex
	nextTxID   uint64
	activeTxns map[types.TxnID]*Transaction
}

func NewTxnManager() *TxnManager {
	return &TxnManager{
		nextTxID:   1,
		activeTxns: make(map[types.TxnID]*Transaction),
	}
}

func (tm *TxnManager) Begin(ctx context.Context) *Transaction {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	id := types.TxnID(atomic.AddUint64(&tm.nextTxID, 1))
	txn := &Transaction{
		ID:    id,
		State: TxnActive,
	}
	tm.activeTxns[id] = txn
	return txn
}

func (tm *TxnManager) Commit(ctx context.Context, txn *Transaction) error {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	if t, ok := tm.activeTxns[txn.ID]; ok {
		t.State = TxnCommitted
	}
	return nil
}

func (tm *TxnManager) Abort(ctx context.Context, txn *Transaction) error {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	if t, ok := tm.activeTxns[txn.ID]; ok {
		t.State = TxnAborted
	}
	return nil
}

func (tm *TxnManager) GetTxnState(id types.TxnID) TxnState {
	tm.mu.RLock()
	defer tm.mu.RUnlock()

	if txn, ok := tm.activeTxns[id]; ok {
		return txn.State
	}

	if uint64(id) < atomic.LoadUint64(&tm.nextTxID) {
		return TxnCommitted
	}
	return TxnAborted
}

// IsVisible checks if a tuple version is visible to the given transaction.
func (tm *TxnManager) IsVisible(txnID types.TxnID, tuple *types.Tuple) bool {
	// 1. Check Creator (Xmin)
	if tuple.Xmin == txnID {
		// Created by us. Check if deleted by us.
		if tuple.Xmax == txnID {
			return false // Created and deleted by us
		}
		return true // Created by us, not deleted
	}

	// Created by someone else. Must be COMMITTED.
	if tm.GetTxnState(tuple.Xmin) != TxnCommitted {
		return false
	}

	// Created by committed. Check if deleted.
	if tuple.Xmax != 0 {
		if tuple.Xmax == txnID {
			return false // Deleted by us
		}
		// Deleted by someone else. If committed, then it's effectively deleted.
		if tm.GetTxnState(tuple.Xmax) == TxnCommitted {
			return false
		}
	}

	return true
}

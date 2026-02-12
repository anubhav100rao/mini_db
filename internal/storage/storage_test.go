package storage

import (
	"context"
	"minidb/internal/txn"
	"minidb/internal/wal"
	"minidb/pkg/types"
	"os"
	"path/filepath"
	"testing"
)

func TestMemTable(t *testing.T) {
	m := NewMemTable()

	t1 := &types.Tuple{Values: []types.Datum{"value1"}}
	t2 := &types.Tuple{Values: []types.Datum{"value2"}}

	m.Put("key1", t1)
	m.Put("key2", t2)

	if val, ok := m.Get("key1"); !ok || val.Values[0] != "value1" {
		t.Error("failed to get key1")
	}

	it := m.Iterator()
	k, v, ok := it.Next()
	if !ok || k != "key1" || v.Values[0] != "value1" {
		t.Errorf("iterator failed 1: %v %v %v", k, v, ok)
	}
	k, v, ok = it.Next()
	if !ok || k != "key2" || v.Values[0] != "value2" {
		t.Errorf("iterator failed 2: %v %v %v", k, v, ok)
	}
	_, _, ok = it.Next()
	if ok {
		t.Error("iterator should end")
	}
}

func TestSSTable(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "sstable_test")
	if err != nil {
		t.Fatal(err)
	}
	path := tmpFile.Name()
	defer os.Remove(path)
	tmpFile.Close()

	// Write
	w, err := NewSSTableWriter(path)
	if err != nil {
		t.Fatal(err)
	}

	t1 := &types.Tuple{Values: []types.Datum{"sstable_val1"}}
	t2 := &types.Tuple{Values: []types.Datum{"sstable_val2"}}

	if err := w.WriteTuple("a", t1); err != nil {
		t.Fatal(err)
	}
	if err := w.WriteTuple("b", t2); err != nil {
		t.Fatal(err)
	}
	w.Close()

	// Read
	r, err := NewSSTableReader(path)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()

	it := r.Iterator()

	k, v, ok := it.Next()
	if !ok {
		t.Fatalf("first next failed: %v", it.Error())
	}
	if k != "a" || v.Values[0] != "sstable_val1" {
		t.Errorf("got %s %v", k, v)
	}

	k, v, ok = it.Next()
	if !ok {
		t.Fatalf("second next failed: %v", it.Error())
	}
	if k != "b" || v.Values[0] != "sstable_val2" {
		t.Errorf("got %s %v", k, v)
	}

	_, _, ok = it.Next()
	if ok {
		t.Error("should be done")
	}
}

func TestLSMEngine(t *testing.T) {
	// Setup dirs
	tmpDir, err := os.MkdirTemp("", "minidb_test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	walFile := tmpDir + "/wal.log"
	w, err := wal.NewWAL(walFile)
	if err != nil {
		t.Fatal(err)
	}

	tm := txn.NewTxnManager()
	engine := NewLSMEngine(tmpDir, w, tm)
	ctx := context.Background()

	tx := tm.Begin(ctx)

	t1 := &types.Tuple{Values: []types.Datum{"row1"}}

	// Test Insert
	// tid=1, tx=tx.ID
	if err := engine.InsertTuple(ctx, 1, t1, tx.ID); err != nil {
		t.Fatal(err)
	}

	// Test Get
	val, err := engine.GetTuple(ctx, 1, "row1", tx.ID)
	if err != nil {
		t.Fatal(err)
	}
	if val.Values[0] != "row1" {
		t.Errorf("expected row1, got %v", val.Values[0])
	}

	// Test Scan
	it, err := engine.ScanTable(ctx, 1, tx.ID)
	if err != nil {
		t.Fatal(err)
	}
	k, v, ok := it.Next()
	if !ok || k != "row1" || v.Values[0] != "row1" {
		t.Errorf("scan failed: %v %v %v", k, v, ok)
	}
	if err := it.Close(); err != nil {
		t.Fatal(err)
	}

	// Test Flush (manual trigger)
	if err := engine.Flush(ctx); err != nil {
		t.Fatal(err)
	}

	// Verify SSTable creation
	files, err := os.ReadDir(tmpDir)
	if err != nil {
		t.Fatal(err)
	}
	sstFound := false
	for _, f := range files {
		if filepath.Ext(f.Name()) == ".sst" {
			sstFound = true
			break
		}
	}
	if !sstFound {
		t.Error("SSTable not created after flush")
	}

	engine.Close()
}

package wal

import (
	"bytes"
	"context"
	"os"
	"testing"
)

func TestWAL(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "wal_test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpFile.Name())
	tmpFile.Close()

	// 1. Open new WAL
	w, err := NewWAL(tmpFile.Name())
	if err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	data1 := []byte("hello")
	data2 := []byte("world")

	// 2. Append data
	if _, err := w.Append(ctx, data1); err != nil {
		t.Fatal(err)
	}
	if _, err := w.Append(ctx, data2); err != nil {
		t.Fatal(err)
	}

	// 3. Close and Reopen to test persistence
	w.Close()

	w2, err := NewWAL(tmpFile.Name())
	if err != nil {
		t.Fatal(err)
	}
	defer w2.Close()

	// 4. Replay
	var replayed [][]byte
	err = w2.Replay(ctx, func(data []byte) error {
		replayed = append(replayed, append([]byte(nil), data...))
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	if len(replayed) != 2 {
		t.Fatalf("expected 2 records, got %d", len(replayed))
	}
	if !bytes.Equal(replayed[0], data1) {
		t.Errorf("expected %s, got %s", data1, replayed[0])
	}
	if !bytes.Equal(replayed[1], data2) {
		t.Errorf("expected %s, got %s", data2, replayed[1])
	}

	// 5. Append more
	data3 := []byte("more")
	if _, err := w2.Append(ctx, data3); err != nil {
		t.Fatal(err)
	}

	// Verify file size or content if needed
}

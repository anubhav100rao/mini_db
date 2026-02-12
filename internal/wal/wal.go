package wal

import (
	"bufio"
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"sync"
)

// WAL manages the write-ahead log.
type WAL struct {
	mu      sync.Mutex
	file    *os.File
	nextLSN uint64 // Log Sequence Number
}

// NewWAL creates or opens a WAL file.
func NewWAL(path string) (*WAL, error) {
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_RDWR, 0644)
	if err != nil {
		return nil, err
	}

	info, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, err
	}

	return &WAL{
		file:    f,
		nextLSN: uint64(info.Size()),
	}, nil
}

// Append writes a record to the WAL and returns its LSN (offset).
// Format: Len(4 bytes) + Data
func (w *WAL) Append(ctx context.Context, data []byte) (uint64, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	// Write length header
	lenBuf := make([]byte, 4)
	binary.BigEndian.PutUint32(lenBuf, uint32(len(data)))

	if _, err := w.file.Write(lenBuf); err != nil {
		return 0, err
	}

	// Write data
	if _, err := w.file.Write(data); err != nil {
		return 0, err
	}

	lsn := w.nextLSN
	w.nextLSN += uint64(4 + len(data))
	return lsn, nil
}

// Flush ensures all writes are persisted to disk.
func (w *WAL) Flush(ctx context.Context) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.file.Sync()
}

// Close closes the WAL file.
func (w *WAL) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.file.Close()
}

// Replay reads the WAL from start to finish.
func (w *WAL) Replay(ctx context.Context, handler func(data []byte) error) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	// Seek to start
	if _, err := w.file.Seek(0, 0); err != nil {
		return err
	}

	// Restore position to end after replay
	defer w.file.Seek(0, 2)

	reader := bufio.NewReader(w.file)

	for {
		// Read length
		lenBuf := make([]byte, 4)
		if _, err := io.ReadFull(reader, lenBuf); err != nil {
			if err == io.EOF {
				break
			}
			return err
		}
		length := binary.BigEndian.Uint32(lenBuf)

		// Read data
		data := make([]byte, length)
		if _, err := io.ReadFull(reader, data); err != nil {
			return err
		}

		if err := handler(data); err != nil {
			return fmt.Errorf("replay handler error: %w", err)
		}
	}

	return nil
}

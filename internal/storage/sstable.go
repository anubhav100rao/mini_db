package storage

import (
	"bufio"
	"encoding/binary"
	"io"
	"os"

	"minidb/pkg/types"
)

// SSTableWriter writes sorted key-values to a file.
type SSTableWriter struct {
	file *os.File
}

func NewSSTableWriter(path string) (*SSTableWriter, error) {
	f, err := os.Create(path)
	if err != nil {
		return nil, err
	}
	return &SSTableWriter{file: f}, nil
}

// Write writes a key-value pair. Keys must be sorted.
// Format: KeyLen(4) + Key + ValLen(4) + Value
func (w *SSTableWriter) WriteTuple(key string, tuple *types.Tuple) error {
	valBytes, err := tuple.Serialize()
	if err != nil {
		return err
	}

	keyLen := uint32(len(key))
	valLen := uint32(len(valBytes))

	// Write Key Length
	if err := binary.Write(w.file, binary.BigEndian, keyLen); err != nil {
		return err
	}
	// Write Key
	if _, err := w.file.WriteString(key); err != nil {
		return err
	}
	// Write Value Length
	if err := binary.Write(w.file, binary.BigEndian, valLen); err != nil {
		return err
	}
	// Write Value
	if _, err := w.file.Write(valBytes); err != nil {
		return err
	}
	return nil
}

func (w *SSTableWriter) Close() error {
	return w.file.Close()
}

// SSTableReader reads an SSTable.
type SSTableReader struct {
	file   *os.File
	reader *bufio.Reader
}

func NewSSTableReader(path string) (*SSTableReader, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	return &SSTableReader{
		file:   f,
		reader: bufio.NewReader(f),
	}, nil
}

func (r *SSTableReader) Close() error {
	return r.file.Close()
}

// Iterator returns an iterator over the SSTable.
func (r *SSTableReader) Iterator() *SSTableIterator {
	// For simplicity, we just scan from current position (start usually).
	// If we need to scan repeatedly, we should Seek(0,0).
	r.file.Seek(0, 0)
	r.reader.Reset(r.file)
	return &SSTableIterator{reader: r.reader}
}

type SSTableIterator struct {
	reader *bufio.Reader
	err    error
}

func (it *SSTableIterator) Next() (string, *types.Tuple, bool) {
	if it.err != nil {
		return "", nil, false
	}

	// Read Key Length
	var keyLen uint32
	if err := binary.Read(it.reader, binary.BigEndian, &keyLen); err != nil {
		if err == io.EOF {
			return "", nil, false
		}
		it.err = err
		return "", nil, false
	}

	// Read Key
	keyBytes := make([]byte, keyLen)
	if _, err := io.ReadFull(it.reader, keyBytes); err != nil {
		it.err = err
		return "", nil, false
	}
	key := string(keyBytes)

	// Read Value Length
	var valLen uint32
	if err := binary.Read(it.reader, binary.BigEndian, &valLen); err != nil {
		it.err = err
		return "", nil, false
	}

	// Read Value
	valBytes := make([]byte, valLen)
	if _, err := io.ReadFull(it.reader, valBytes); err != nil {
		it.err = err
		return "", nil, false
	}

	tuple, err := types.DeserializeTuple(valBytes)
	if err != nil {
		it.err = err
		return "", nil, false
	}

	return key, tuple, true
}

func (it *SSTableIterator) Error() error {
	return it.err
}

func (it *SSTableIterator) Close() error {
	return nil
}

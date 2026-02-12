package types

import (
	"bytes"
	"encoding/gob"
	"fmt"
	"io"
)

// Basic Identifier Types
type TableID uint32
type TxnID uint64
type PageID uint64

// Datum represents a value in a tuple.
// For now, we use interface{}, but later we can make this more type-safe.
type Datum interface{}

// Tuple represents a row in the database.
type Tuple struct {
	Values []Datum
	// MVCC Header
	Xmin TxnID
	Xmax TxnID
}

// Schema defines the structure of a table.
type Schema struct {
	ColNames []string
	ColTypes []string // simplified: "int", "string", etc.
}

// Common errors
var ErrEOF = io.EOF
var ErrTableNotFound = fmt.Errorf("table not found")
var ErrTxnAborted = fmt.Errorf("transaction aborted")

// Serialize encodes the tuple into bytes (using gob for simplicity).
func (t *Tuple) Serialize() ([]byte, error) {
	var buf bytes.Buffer
	enc := gob.NewEncoder(&buf)
	if err := enc.Encode(t.Values); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// DeserializeTuple decoding a tuple from bytes.
func DeserializeTuple(data []byte) (*Tuple, error) {
	var values []Datum
	buf := bytes.NewBuffer(data)
	dec := gob.NewDecoder(buf)
	if err := dec.Decode(&values); err != nil {
		return nil, err
	}
	return &Tuple{Values: values}, nil
}

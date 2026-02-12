package parser

import (
	"testing"
)

func TestParseSelect(t *testing.T) {
	sql := "SELECT * FROM users WHERE id = 123"
	stmt, err := Parse(sql)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	sel, ok := stmt.(*SelectStmt)
	if !ok {
		t.Fatalf("Expected SelectStmt, got %T", stmt)
	}

	if sel.TableName != "users" {
		t.Errorf("Expected table users, got %s", sel.TableName)
	}

	if sel.Where == nil {
		t.Fatal("Expected Where clause, got nil")
	}

	if sel.Where.Column != "id" {
		t.Errorf("Expected column id, got %s", sel.Where.Column)
	}
	if sel.Where.Op != "=" {
		t.Errorf("Expected op =, got %s", sel.Where.Op)
	}
	if val, ok := sel.Where.Value.(int); !ok || val != 123 {
		t.Errorf("Expected value 123, got %v", sel.Where.Value)
	}
}

func TestParseInsert(t *testing.T) {
	sql := "INSERT INTO users VALUES (1, 'alice'), (2, 'bob')"
	stmt, err := Parse(sql)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	ins, ok := stmt.(*InsertStmt)
	if !ok {
		t.Fatalf("Expected InsertStmt, got %T", stmt)
	}

	if ins.TableName != "users" {
		t.Errorf("Expected table users, got %s", ins.TableName)
	}

	if len(ins.Values) != 2 {
		t.Errorf("Expected 2 rows, got %d", len(ins.Values))
	}

	row1 := ins.Values[0]
	if row1[0].(int) != 1 || row1[1].(string) != "alice" {
		t.Errorf("Row 1 mismatch: %v", row1)
	}
}

func TestParseErrors(t *testing.T) {
	tests := []string{
		"SELECT * FROM",
		"INSERT INTO users VALUES",
		"SELECT * FROM users WHERE id =",
		"INVALID SQL",
	}

	for _, sql := range tests {
		if _, err := Parse(sql); err == nil {
			t.Errorf("Expected error for SQL: %s", sql)
		}
	}
}

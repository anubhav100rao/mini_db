package parser

type Statement interface {
	statement()
}

type SelectStmt struct {
	TableName string
	Columns   []string // empty means *
	Where     *WhereClause
}

type WhereClause struct {
	Column string
	Op     string
	Value  interface{}
}

func (s *SelectStmt) statement() {}

type InsertStmt struct {
	TableName string
	Values    [][]interface{} // int or string
}

func (s *InsertStmt) statement() {}

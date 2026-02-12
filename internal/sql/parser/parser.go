package parser

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"unicode"
)

// Parse parses a SQL string into an AST.
func Parse(sql string) (Statement, error) {
	p := &parser{input: sql}
	return p.parse()
}

type parser struct {
	input string
	pos   int
}

func (p *parser) parse() (Statement, error) {
	p.skipWhitespace()
	if p.pos >= len(p.input) {
		return nil, errors.New("empty statement")
	}

	keyword := p.parseIdentifier()
	switch strings.ToUpper(keyword) {
	case "SELECT":
		return p.parseSelect()
	case "INSERT":
		return p.parseInsert()
	default:
		return nil, fmt.Errorf("unexpected keyword: %s", keyword)
	}
}

func (p *parser) skipWhitespace() {
	for p.pos < len(p.input) && unicode.IsSpace(rune(p.input[p.pos])) {
		p.pos++
	}
}

func (p *parser) parseIdentifier() string {
	start := p.pos
	for p.pos < len(p.input) {
		r := rune(p.input[p.pos])
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '_' {
			break
		}
		p.pos++
	}
	return p.input[start:p.pos]
}

func (p *parser) expect(keyword string) error {
	p.skipWhitespace()
	id := p.parseIdentifier()
	if !strings.EqualFold(id, keyword) {
		return fmt.Errorf("expected %s, got %s", keyword, id)
	}
	return nil
}

// SELECT * FROM table [WHERE col = val]
func (p *parser) parseSelect() (*SelectStmt, error) {
	stmt := &SelectStmt{}

	p.skipWhitespace()
	if p.pos < len(p.input) && p.input[p.pos] == '*' {
		p.pos++
	} else {
		return nil, errors.New("only SELECT * supported currently")
	}

	if err := p.expect("FROM"); err != nil {
		return nil, err
	}

	p.skipWhitespace()
	stmt.TableName = p.parseIdentifier()
	if stmt.TableName == "" {
		return nil, errors.New("expected table name")
	}

	// Optional WHERE
	p.skipWhitespace()
	if p.pos < len(p.input) {
		// Lookahead for WHERE
		currentPos := p.pos
		id := p.parseIdentifier()
		if strings.ToUpper(id) == "WHERE" {
			where, err := p.parseWhere()
			if err != nil {
				return nil, err
			}
			stmt.Where = where
		} else {
			// Not WHERE, maybe something else?
			// Reset pos?
			p.pos = currentPos // actually we should just stop if EOF or unexpected
		}
	}

	return stmt, nil
}

func (p *parser) parseWhere() (*WhereClause, error) {
	p.skipWhitespace()
	col := p.parseIdentifier()
	if col == "" {
		return nil, errors.New("expected column in WHERE")
	}

	p.skipWhitespace()
	// Parse Op (= only for now)
	// FIX: use < to check bound, not >=
	if p.pos < len(p.input) && p.input[p.pos] == '=' {
		p.pos++
	} else {
		return nil, errors.New("expected '=' in WHERE")
	}

	p.skipWhitespace()
	// Parse Value
	var val interface{}
	if p.pos >= len(p.input) {
		return nil, errors.New("unexpected EOF in WHERE value")
	}

	if p.input[p.pos] == '\'' {
		// String
		p.pos++
		start := p.pos
		for p.pos < len(p.input) && p.input[p.pos] != '\'' {
			p.pos++
		}
		val = p.input[start:p.pos]
		p.pos++ // skip closing quote
	} else if unicode.IsDigit(rune(p.input[p.pos])) {
		// Int
		start := p.pos
		for p.pos < len(p.input) && unicode.IsDigit(rune(p.input[p.pos])) {
			p.pos++
		}
		i, _ := strconv.Atoi(p.input[start:p.pos])
		val = i
	} else {
		return nil, fmt.Errorf("unexpected char in WHERE value: %c", p.input[p.pos])
	}

	return &WhereClause{Column: col, Op: "=", Value: val}, nil
}

// INSERT INTO table VALUES (1, 'a'), (2, 'b')
func (p *parser) parseInsert() (*InsertStmt, error) {
	if err := p.expect("INTO"); err != nil {
		return nil, err
	}

	p.skipWhitespace()
	tableName := p.parseIdentifier()

	if err := p.expect("VALUES"); err != nil {
		return nil, err
	}

	var values [][]interface{}

	for {
		p.skipWhitespace()
		if p.pos >= len(p.input) || p.input[p.pos] != '(' {
			break
		}
		p.pos++ // skip '('

		var row []interface{}
		for {
			p.skipWhitespace()
			if p.pos >= len(p.input) {
				return nil, errors.New("unexpected EOF in VALUES")
			}

			// Parse Value (Int or String)
			if p.input[p.pos] == '\'' {
				// String
				p.pos++
				start := p.pos
				for p.pos < len(p.input) && p.input[p.pos] != '\'' {
					p.pos++
				}
				row = append(row, p.input[start:p.pos])
				p.pos++
			} else if unicode.IsDigit(rune(p.input[p.pos])) {
				// Int
				start := p.pos
				for p.pos < len(p.input) && unicode.IsDigit(rune(p.input[p.pos])) {
					p.pos++
				}
				val, _ := strconv.Atoi(p.input[start:p.pos])
				row = append(row, val)
			} else {
				return nil, fmt.Errorf("unexpected char in value: %c", p.input[p.pos])
			}

			p.skipWhitespace()
			if p.pos < len(p.input) && p.input[p.pos] == ',' {
				p.pos++
			} else {
				break
			}
		}

		if p.pos >= len(p.input) || p.input[p.pos] != ')' {
			return nil, errors.New("expected ')'")
		}
		p.pos++
		values = append(values, row)

		p.skipWhitespace()
		if p.pos < len(p.input) && p.input[p.pos] == ',' {
			p.pos++
		} else {
			break
		}
	}

	if len(values) == 0 {
		return nil, errors.New("expected values")
	}
	return &InsertStmt{TableName: tableName, Values: values}, nil
}

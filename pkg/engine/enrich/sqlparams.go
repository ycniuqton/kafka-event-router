package enrich

import (
	"errors"
	"strconv"
	"strings"
	"unicode"
)

// SQLSyntax holds the characters the parameter rewriter recognises.
type SQLSyntax byte

const (
	SQLParamMark    SQLSyntax = ':'
	SQLStringQuote  SQLSyntax = '\''
	SQLIdentQuote   SQLSyntax = '"'
	SQLStatementEnd SQLSyntax = ';'
	SQLPositional   SQLSyntax = '$'
	SQLIdentJoiner  SQLSyntax = '_'
)

// SQLReadVerbs are the statement starts accepted by sql lookups (the source also enforces READ ONLY).
var SQLReadVerbs = []string{"select", "with"}

// SQL statement errors raised at compile time.
var (
	ErrSQLNotSelect      = errors.New("only one SELECT (or WITH … SELECT) statement is allowed")
	ErrSQLMultiStatement = errors.New("only one statement is allowed")
	ErrSQLPositional     = errors.New("use :name parameters, not $n")
)

// SQLStatement is a query rewritten to positional parameters plus the ordered parameter names.
type SQLStatement struct {
	Text  string
	Names []string
}

// SQLParams rewrites `:name` parameters to `$n` outside quotes; `::` casts are kept.
type SQLParams struct{}

// Rewrite validates the statement shape and returns the positional form.
func (SQLParams) Rewrite(sql string) (SQLStatement, error) {
	trimmed := strings.TrimRight(strings.TrimSpace(sql), string(SQLStatementEnd))
	if !(SQLParams{}).readVerb(trimmed) {
		return SQLStatement{}, ErrSQLNotSelect
	}
	scan := &sqlScanner{src: trimmed, index: map[string]int{}}
	if err := scan.run(); err != nil {
		return SQLStatement{}, err
	}
	return SQLStatement{Text: scan.out.String(), Names: scan.names}, nil
}

func (SQLParams) readVerb(sql string) bool {
	lower := strings.ToLower(sql)
	for _, verb := range SQLReadVerbs {
		if strings.HasPrefix(lower, verb) {
			return true
		}
	}
	return false
}

// sqlScanner walks one statement, copying text and numbering parameters.
type sqlScanner struct {
	src   string
	pos   int
	out   strings.Builder
	names []string
	index map[string]int
	quote byte
}

func (s *sqlScanner) run() error {
	for s.pos < len(s.src) {
		if err := s.step(); err != nil {
			return err
		}
	}
	return nil
}

func (s *sqlScanner) step() error {
	c := s.src[s.pos]
	switch {
	case s.quote != 0:
		s.copyQuoted(c)
	case c == byte(SQLStringQuote) || c == byte(SQLIdentQuote):
		s.quote = c
		s.copy(1)
	case c == byte(SQLStatementEnd):
		return ErrSQLMultiStatement
	case c == byte(SQLPositional) && s.digitAt(s.pos+1):
		return ErrSQLPositional
	case c == byte(SQLParamMark):
		s.param()
	default:
		s.copy(1)
	}
	return nil
}

func (s *sqlScanner) copyQuoted(c byte) {
	if c == s.quote {
		s.quote = 0
	}
	s.copy(1)
}

func (s *sqlScanner) copy(n int) {
	s.out.WriteString(s.src[s.pos : s.pos+n])
	s.pos += n
}

// param handles `::` casts and `:name` parameters.
func (s *sqlScanner) param() {
	if s.pos+1 < len(s.src) && s.src[s.pos+1] == byte(SQLParamMark) {
		s.copy(2)
		return
	}
	end := s.pos + 1
	for end < len(s.src) && s.identByte(s.src[end], end == s.pos+1) {
		end++
	}
	if end == s.pos+1 {
		s.copy(1)
		return
	}
	name := s.src[s.pos+1 : end]
	n, seen := s.index[name]
	if !seen {
		s.names = append(s.names, name)
		n = len(s.names)
		s.index[name] = n
	}
	s.out.WriteString(string(SQLPositional) + strconv.Itoa(n))
	s.pos = end
}

func (s *sqlScanner) identByte(c byte, first bool) bool {
	letter := c == byte(SQLIdentJoiner) || unicode.IsLetter(rune(c))
	return letter || (!first && unicode.IsDigit(rune(c)))
}

func (s *sqlScanner) digitAt(i int) bool {
	return i < len(s.src) && unicode.IsDigit(rune(s.src[i]))
}

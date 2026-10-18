// Package formula parses and evaluates the route formula language, e.g.
// `concat(value.first, " ", value.last)` or `round(value.price * 1.1, 2)`, into an AST
// of native operators and catalogue functions, compiled once per route version.
package formula

import (
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"
)

// TokenKind classifies lexer tokens.
type TokenKind string

const (
	TokNumber   TokenKind = "number"
	TokString   TokenKind = "string"
	TokIdent    TokenKind = "ident"
	TokSymbol   TokenKind = "symbol"
	TokLParen   TokenKind = "("
	TokRParen   TokenKind = ")"
	TokLBracket TokenKind = "["
	TokRBracket TokenKind = "]"
	TokComma    TokenKind = ","
	TokDot      TokenKind = "."
	TokEOF      TokenKind = "eof"
)

// Symbol is an operator spelling.
type Symbol string

const (
	SymPlus    Symbol = "+"
	SymMinus   Symbol = "-"
	SymStar    Symbol = "*"
	SymSlash   Symbol = "/"
	SymPercent Symbol = "%"
	SymEq      Symbol = "=="
	SymNe      Symbol = "!="
	SymLt      Symbol = "<"
	SymLte     Symbol = "<="
	SymGt      Symbol = ">"
	SymGte     Symbol = ">="
	SymAnd     Symbol = "&&"
	SymOr      Symbol = "||"
	SymNot     Symbol = "!"
)

// Limit bounds formula size so hostile input cannot exhaust the compiler.
type Limit int

const (
	LimitMaxLength Limit = 4096
	LimitMaxDepth  Limit = 64
)

// Quote and escape characters of string literals.
const (
	QuoteDouble rune = '"'
	QuoteSingle rune = '\''
	EscapeChar  rune = '\\'
)

// escapes maps the character after a backslash to its value.
var escapes = map[rune]rune{'n': '\n', 't': '\t', 'r': '\r', '\\': '\\', '"': '"', '\'': '\''}

// twoCharSymbols are matched before single-character symbols.
var twoCharSymbols = []Symbol{SymEq, SymNe, SymLte, SymGte, SymAnd, SymOr}

// oneCharSymbols are single-character operators.
var oneCharSymbols = []Symbol{SymPlus, SymMinus, SymStar, SymSlash, SymPercent, SymLt, SymGt, SymNot}

// punctuation maps single characters to their token kinds.
var punctuation = map[rune]TokenKind{'(': TokLParen, ')': TokRParen, '[': TokLBracket, ']': TokRBracket, ',': TokComma, '.': TokDot}

// Token is one lexeme with its byte offset.
type Token struct {
	Kind TokenKind
	Text string
	Pos  int
}

// SyntaxError is a compile failure at a byte offset of the formula.
type SyntaxError struct {
	Pos int
	Msg string
}

func (e *SyntaxError) Error() string { return fmt.Sprintf("%s at position %d", e.Msg, e.Pos) }

// Lexer splits a formula into tokens.
type Lexer struct {
	src string
	pos int
}

// Tokens returns all tokens ending with TokEOF.
func (l *Lexer) Tokens(src string) ([]Token, error) {
	l.src, l.pos = src, 0
	if len(src) > int(LimitMaxLength) {
		return nil, &SyntaxError{Pos: 0, Msg: fmt.Sprintf("formula longer than %d bytes", LimitMaxLength)}
	}
	out := []Token{}
	for {
		l.skipSpace()
		if l.pos >= len(l.src) {
			return append(out, Token{Kind: TokEOF, Pos: l.pos}), nil
		}
		t, err := l.next()
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
}

func (l *Lexer) skipSpace() {
	for l.pos < len(l.src) {
		r, size := utf8.DecodeRuneInString(l.src[l.pos:])
		if !unicode.IsSpace(r) {
			return
		}
		l.pos += size
	}
}

// next scans one token at the current position.
func (l *Lexer) next() (Token, error) {
	r, _ := utf8.DecodeRuneInString(l.src[l.pos:])
	switch {
	case r == QuoteDouble || r == QuoteSingle:
		return l.str(r)
	case r >= '0' && r <= '9':
		return l.number(), nil
	case r == '_' || unicode.IsLetter(r):
		return l.ident(), nil
	}
	if k, ok := punctuation[r]; ok {
		l.pos++
		return Token{Kind: k, Text: string(r), Pos: l.pos - 1}, nil
	}
	return l.symbol()
}

func (l *Lexer) symbol() (Token, error) {
	for _, set := range [][]Symbol{twoCharSymbols, oneCharSymbols} {
		for _, s := range set {
			if strings.HasPrefix(l.src[l.pos:], string(s)) {
				start := l.pos
				l.pos += len(s)
				return Token{Kind: TokSymbol, Text: string(s), Pos: start}, nil
			}
		}
	}
	return Token{}, &SyntaxError{Pos: l.pos, Msg: fmt.Sprintf("unexpected character %q", l.src[l.pos:l.pos+1])}
}

func (l *Lexer) number() Token {
	start := l.pos
	seenDot := false
	for l.pos < len(l.src) {
		c := l.src[l.pos]
		dotOK := c == '.' && !seenDot && l.pos+1 < len(l.src) && l.src[l.pos+1] >= '0' && l.src[l.pos+1] <= '9'
		if !(c >= '0' && c <= '9') && !dotOK {
			break
		}
		seenDot = seenDot || c == '.'
		l.pos++
	}
	return Token{Kind: TokNumber, Text: l.src[start:l.pos], Pos: start}
}

func (l *Lexer) ident() Token {
	start := l.pos
	for l.pos < len(l.src) {
		r, size := utf8.DecodeRuneInString(l.src[l.pos:])
		if r != '_' && !unicode.IsLetter(r) && !unicode.IsDigit(r) {
			break
		}
		l.pos += size
	}
	return Token{Kind: TokIdent, Text: l.src[start:l.pos], Pos: start}
}

// str scans a quoted literal with backslash escapes.
func (l *Lexer) str(quote rune) (Token, error) {
	start := l.pos
	l.pos++
	var b strings.Builder
	for l.pos < len(l.src) {
		r, size := utf8.DecodeRuneInString(l.src[l.pos:])
		l.pos += size
		if r == quote {
			return Token{Kind: TokString, Text: b.String(), Pos: start}, nil
		}
		if r != EscapeChar {
			b.WriteRune(r)
			continue
		}
		esc, ok := l.escape()
		if !ok {
			return Token{}, &SyntaxError{Pos: l.pos, Msg: "invalid escape sequence"}
		}
		b.WriteRune(esc)
	}
	return Token{}, &SyntaxError{Pos: start, Msg: "unterminated string"}
}

func (l *Lexer) escape() (rune, bool) {
	if l.pos >= len(l.src) {
		return 0, false
	}
	r, size := utf8.DecodeRuneInString(l.src[l.pos:])
	l.pos += size
	v, ok := escapes[r]
	return v, ok
}

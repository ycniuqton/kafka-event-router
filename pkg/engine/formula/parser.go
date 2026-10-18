package formula

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/eventrouter/router/pkg/engine/path"
)

// Keyword is a reserved literal identifier.
type Keyword string

const (
	KeywordTrue  Keyword = "true"
	KeywordFalse Keyword = "false"
	KeywordNull  Keyword = "null"
)

// keywords maps literal identifiers to values.
var keywords = map[Keyword]any{KeywordTrue: true, KeywordFalse: false, KeywordNull: nil}

// Compiler parses formulas against an operator set and a function catalogue.
type Compiler struct {
	Functions *FunctionCatalog
	Operators *OperatorSet
}

// NewDefaultCompiler uses the built-in operators and functions.
func NewDefaultCompiler() *Compiler {
	return &Compiler{Functions: DefaultFunctionCatalog(), Operators: DefaultOperatorSet()}
}

// Compile parses src; failures are *SyntaxError with a byte position.
func (c *Compiler) Compile(src string) (*Program, error) {
	toks, err := (&Lexer{}).Tokens(src)
	if err != nil {
		return nil, err
	}
	p := &parser{toks: toks, c: c}
	root, err := p.expr(PrecLowest)
	if err != nil {
		return nil, err
	}
	if t := p.peek(); t.Kind != TokEOF {
		return nil, &SyntaxError{Pos: t.Pos, Msg: fmt.Sprintf("unexpected %q", t.Text)}
	}
	return &Program{root: root, src: src}, nil
}

// parser is a precedence-climbing parser over a token list.
type parser struct {
	toks  []Token
	pos   int
	depth int
	c     *Compiler
}

func (p *parser) peek() Token { return p.toks[p.pos] }

func (p *parser) advance() Token {
	t := p.toks[p.pos]
	if t.Kind != TokEOF {
		p.pos++
	}
	return t
}

func (p *parser) expect(k TokenKind) (Token, error) {
	t := p.advance()
	if t.Kind != k {
		return t, &SyntaxError{Pos: t.Pos, Msg: fmt.Sprintf("expected %q, got %q", k, t.Text)}
	}
	return t, nil
}

// expr parses binary operators with precedence above minPrec.
func (p *parser) expr(minPrec Precedence) (Expr, error) {
	p.depth++
	defer func() { p.depth-- }()
	if p.depth > int(LimitMaxDepth) {
		return nil, &SyntaxError{Pos: p.peek().Pos, Msg: "formula nested too deeply"}
	}
	left, err := p.unary()
	if err != nil {
		return nil, err
	}
	for {
		t := p.peek()
		op, ok := p.c.Operators.Binary(Symbol(t.Text))
		if t.Kind != TokSymbol || !ok || op.Precedence() <= minPrec {
			return left, nil
		}
		p.advance()
		right, err := p.expr(op.Precedence())
		if err != nil {
			return nil, err
		}
		left = binary{op: op, left: left, right: right}
	}
}

// unary parses prefix operators.
func (p *parser) unary() (Expr, error) {
	t := p.peek()
	if t.Kind != TokSymbol {
		return p.primary()
	}
	op, ok := p.c.Operators.Unary(Symbol(t.Text))
	if !ok {
		return nil, &SyntaxError{Pos: t.Pos, Msg: fmt.Sprintf("unexpected %q", t.Text)}
	}
	p.advance()
	operand, err := p.expr(PrecMultiply)
	if err != nil {
		return nil, err
	}
	return unary{op: op, operand: operand}, nil
}

// primary parses literals, parentheses, calls and field paths.
func (p *parser) primary() (Expr, error) {
	t := p.advance()
	switch t.Kind {
	case TokNumber:
		return p.number(t)
	case TokString:
		return literal{value: t.Text}, nil
	case TokLParen:
		inner, err := p.expr(PrecLowest)
		if err != nil {
			return nil, err
		}
		_, err = p.expect(TokRParen)
		return inner, err
	case TokIdent:
		return p.identifier(t)
	}
	return nil, &SyntaxError{Pos: t.Pos, Msg: fmt.Sprintf("unexpected %q", p.describe(t))}
}

func (p *parser) describe(t Token) string {
	if t.Kind == TokEOF {
		return string(TokEOF)
	}
	return t.Text
}

func (p *parser) number(t Token) (Expr, error) {
	if i, err := strconv.ParseInt(t.Text, 10, 64); err == nil {
		return literal{value: i}, nil
	}
	f, err := strconv.ParseFloat(t.Text, 64)
	if err != nil {
		return nil, &SyntaxError{Pos: t.Pos, Msg: fmt.Sprintf("invalid number %q", t.Text)}
	}
	return literal{value: f}, nil
}

// identifier resolves keywords, function calls and field paths.
func (p *parser) identifier(t Token) (Expr, error) {
	if v, ok := keywords[Keyword(t.Text)]; ok {
		return literal{value: v}, nil
	}
	if p.peek().Kind == TokLParen {
		return p.call(t)
	}
	return p.fieldPath(t)
}

// call parses an argument list and checks the function and its arity.
func (p *parser) call(name Token) (Expr, error) {
	fn, ok := p.c.Functions.Get(FuncName(name.Text))
	if !ok {
		return nil, &SyntaxError{Pos: name.Pos, Msg: fmt.Sprintf("unknown function %q", name.Text)}
	}
	p.advance()
	args := []Expr{}
	for p.peek().Kind != TokRParen {
		if len(args) > 0 {
			if _, err := p.expect(TokComma); err != nil {
				return nil, err
			}
		}
		a, err := p.expr(PrecLowest)
		if err != nil {
			return nil, err
		}
		args = append(args, a)
	}
	p.advance()
	if !fn.Arity().Accepts(len(args)) {
		return nil, &SyntaxError{Pos: name.Pos, Msg: (&ArityError{Func: fn.Name(), Arity: fn.Arity(), Got: len(args)}).Error()}
	}
	return call{fn: fn, args: args}, nil
}

// fieldPath joins `root.member[0].member` tokens and parses them with the engine path grammar.
func (p *parser) fieldPath(root Token) (Expr, error) {
	var b strings.Builder
	b.WriteString(root.Text)
	for {
		seg, ok, err := p.segment()
		if err != nil {
			return nil, err
		}
		if !ok {
			break
		}
		b.WriteString(seg)
	}
	fp, err := path.Parser{}.Parse(b.String())
	if err != nil {
		return nil, &SyntaxError{Pos: root.Pos, Msg: err.Error()}
	}
	return fieldRef{path: fp}, nil
}

// segment consumes one `.member` or `[index]` suffix.
func (p *parser) segment() (string, bool, error) {
	switch p.peek().Kind {
	case TokDot:
		p.advance()
		name, err := p.expect(TokIdent)
		return string(path.SyntaxDot) + name.Text, err == nil, err
	case TokLBracket:
		p.advance()
		idx, err := p.expect(TokNumber)
		if err != nil {
			return "", false, err
		}
		_, err = p.expect(TokRBracket)
		return string(path.SyntaxIndexOpen) + idx.Text + string(path.SyntaxIndexClose), err == nil, err
	}
	return "", false, nil
}

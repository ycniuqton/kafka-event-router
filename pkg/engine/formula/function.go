package formula

import (
	"fmt"
)

// FuncName is the spelling of a catalogue function.
type FuncName string

const (
	FuncConcat     FuncName = "concat"
	FuncUpper      FuncName = "upper"
	FuncLower      FuncName = "lower"
	FuncTrim       FuncName = "trim"
	FuncSubstring  FuncName = "substring"
	FuncLen        FuncName = "len"
	FuncRound      FuncName = "round"
	FuncCoalesce   FuncName = "coalesce"
	FuncIf         FuncName = "if"
	FuncToNumber   FuncName = "toNumber"
	FuncToString   FuncName = "toString"
	FuncNow        FuncName = "now"
	FuncFormatDate FuncName = "formatDate"
	FuncMask       FuncName = "mask"
	FuncHash       FuncName = "hash"
	FuncLookup     FuncName = "lookup"
)

// FunctionCategory groups functions in the Studio autocomplete.
type FunctionCategory string

const (
	CategoryText    FunctionCategory = "text"
	CategoryNumber  FunctionCategory = "number"
	CategoryLogic   FunctionCategory = "logic"
	CategoryDate    FunctionCategory = "date"
	CategoryPrivacy FunctionCategory = "privacy"
	CategoryLookup  FunctionCategory = "lookup"
)

// ValueKind names argument and result types in signatures.
type ValueKind string

const (
	KindAny     ValueKind = "any"
	KindString  ValueKind = "string"
	KindNumber  ValueKind = "number"
	KindInteger ValueKind = "integer"
	KindBoolean ValueKind = "boolean"
)

// ArityVariadic marks a function without an upper argument bound.
const ArityVariadic = -1

// Arity is the accepted argument count range.
type Arity struct {
	Min int `json:"min"`
	Max int `json:"max"`
}

// Accepts reports whether n arguments are allowed.
func (a Arity) Accepts(n int) bool {
	return n >= a.Min && (a.Max == ArityVariadic || n <= a.Max)
}

// Param documents one parameter.
type Param struct {
	Name     string    `json:"name"`
	Kind     ValueKind `json:"kind"`
	Optional bool      `json:"optional,omitempty"`
	Variadic bool      `json:"variadic,omitempty"`
}

// Signature documents a function for the engine catalogue.
type Signature struct {
	Params      []Param          `json:"params"`
	Returns     ValueKind        `json:"returns"`
	Category    FunctionCategory `json:"category"`
	Description string           `json:"description"`
	Example     string           `json:"example"`
}

// Function is one catalogue function; arguments are evaluated lazily on demand.
type Function interface {
	Name() FuncName
	Arity() Arity
	Signature() Signature
	Call(args *Args) (any, error)
}

// FunctionCatalog is the registry of formula functions selected by name.
type FunctionCatalog struct {
	funcs map[FuncName]Function
	order []FuncName
}

// NewFunctionCatalog registers the given functions.
func NewFunctionCatalog(fs ...Function) *FunctionCatalog {
	c := &FunctionCatalog{funcs: map[FuncName]Function{}}
	for _, f := range fs {
		c.funcs[f.Name()] = f
		c.order = append(c.order, f.Name())
	}
	return c
}

// DefaultFunctionCatalog returns every built-in function.
func DefaultFunctionCatalog() *FunctionCatalog {
	return NewFunctionCatalog(
		Concat{}, Upper{}, Lower{}, Trim{}, Substring{}, Len{},
		Round{}, ToNumber{}, ToString{},
		Coalesce{}, If{},
		Now{}, FormatDate{},
		Mask{}, Hash{}, Lookup{},
	)
}

// Get returns the function of name.
func (c *FunctionCatalog) Get(name FuncName) (Function, bool) {
	f, ok := c.funcs[name]
	return f, ok
}

// All returns the functions in registration order.
func (c *FunctionCatalog) All() []Function {
	out := make([]Function, 0, len(c.order))
	for _, n := range c.order {
		out = append(out, c.funcs[n])
	}
	return out
}

// ArityError reports a call with an unsupported number of arguments.
type ArityError struct {
	Func  FuncName
	Arity Arity
	Got   int
}

func (e *ArityError) Error() string {
	if e.Arity.Max == ArityVariadic {
		return fmt.Sprintf("%s() takes at least %d arguments, got %d", e.Func, e.Arity.Min, e.Got)
	}
	return fmt.Sprintf("%s() takes %d to %d arguments, got %d", e.Func, e.Arity.Min, e.Arity.Max, e.Got)
}

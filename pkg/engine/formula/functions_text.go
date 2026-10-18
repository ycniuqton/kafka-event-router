package formula

import (
	"strings"
	"unicode/utf8"
)

// Concat joins the text of every argument; null contributes nothing.
type Concat struct{}

func (Concat) Name() FuncName { return FuncConcat }
func (Concat) Arity() Arity   { return Arity{Min: 1, Max: ArityVariadic} }
func (Concat) Signature() Signature {
	return Signature{Params: []Param{{Name: "values", Kind: KindAny, Variadic: true}}, Returns: KindString,
		Category: CategoryText, Description: "Joins values as text.", Example: `concat(value.first, " ", value.last)`}
}

func (Concat) Call(args *Args) (any, error) {
	vals, err := args.All()
	if err != nil {
		return nil, err
	}
	var b strings.Builder
	for _, v := range vals {
		b.WriteString(Text{}.Of(v))
	}
	return b.String(), nil
}

// TextFunc applies a string transform to one argument; null stays null.
type TextFunc struct {
	name  FuncName
	apply func(string) string
	doc   string
}

func (f TextFunc) Name() FuncName { return f.name }
func (TextFunc) Arity() Arity     { return Arity{Min: 1, Max: 1} }
func (f TextFunc) Signature() Signature {
	return Signature{Params: []Param{{Name: "text", Kind: KindString}}, Returns: KindString,
		Category: CategoryText, Description: f.doc, Example: string(f.name) + "(value.name)"}
}

func (f TextFunc) Call(args *Args) (any, error) {
	v, err := args.At(0)
	if err != nil || v == nil {
		return nil, err
	}
	return f.apply(Text{}.Of(v)), nil
}

// Upper upper-cases text.
type Upper struct{ TextFunc }

// Lower lower-cases text.
type Lower struct{ TextFunc }

// Trim removes surrounding white space.
type Trim struct{ TextFunc }

func (Upper) Name() FuncName { return FuncUpper }
func (Lower) Name() FuncName { return FuncLower }
func (Trim) Name() FuncName  { return FuncTrim }

func (Upper) Call(args *Args) (any, error) {
	return TextFunc{name: FuncUpper, apply: strings.ToUpper}.Call(args)
}
func (Lower) Call(args *Args) (any, error) {
	return TextFunc{name: FuncLower, apply: strings.ToLower}.Call(args)
}
func (Trim) Call(args *Args) (any, error) {
	return TextFunc{name: FuncTrim, apply: strings.TrimSpace}.Call(args)
}

func (Upper) Signature() Signature {
	return TextFunc{name: FuncUpper, doc: "Converts text to upper case."}.Signature()
}
func (Lower) Signature() Signature {
	return TextFunc{name: FuncLower, doc: "Converts text to lower case."}.Signature()
}
func (Trim) Signature() Signature {
	return TextFunc{name: FuncTrim, doc: "Removes leading and trailing white space."}.Signature()
}

// Substring returns `length` characters from `start` (0-based, clamped to the text).
type Substring struct{}

func (Substring) Name() FuncName { return FuncSubstring }
func (Substring) Arity() Arity   { return Arity{Min: 2, Max: 3} }
func (Substring) Signature() Signature {
	return Signature{Params: []Param{{Name: "text", Kind: KindString}, {Name: "start", Kind: KindInteger}, {Name: "length", Kind: KindInteger, Optional: true}},
		Returns: KindString, Category: CategoryText, Description: "Part of the text by character position.", Example: "substring(value.iban, 0, 4)"}
}

func (Substring) Call(args *Args) (any, error) {
	vals, err := args.All()
	if err != nil || vals[0] == nil {
		return nil, err
	}
	runes := []rune(Text{}.Of(vals[0]))
	start, err := Numbers{}.Of(vals[1])
	if err != nil {
		return nil, err
	}
	from := Bounds{}.Clamp(int(start), len(runes))
	to := len(runes)
	if len(vals) > 2 {
		n, err := Numbers{}.Of(vals[2])
		if err != nil {
			return nil, err
		}
		to = Bounds{}.Clamp(from+int(n), len(runes))
	}
	return string(runes[from:max(from, to)]), nil
}

// Bounds clamps indexes into [0, n].
type Bounds struct{}

// Clamp limits i to the valid range.
func (Bounds) Clamp(i, n int) int { return min(max(i, 0), n) }

// Len counts characters of text, items of arrays or members of objects; null is 0.
type Len struct{}

func (Len) Name() FuncName { return FuncLen }
func (Len) Arity() Arity   { return Arity{Min: 1, Max: 1} }
func (Len) Signature() Signature {
	return Signature{Params: []Param{{Name: "value", Kind: KindAny}}, Returns: KindInteger,
		Category: CategoryText, Description: "Length of text, list or object.", Example: "len(value.items)"}
}

func (Len) Call(args *Args) (any, error) {
	v, err := args.At(0)
	if err != nil {
		return nil, err
	}
	switch t := v.(type) {
	case nil:
		return int64(0), nil
	case []any:
		return int64(len(t)), nil
	case map[string]any:
		return int64(len(t)), nil
	}
	return int64(utf8.RuneCountInString(Text{}.Of(v))), nil
}

// ToString renders any value as text; null stays null.
type ToString struct{}

func (ToString) Name() FuncName { return FuncToString }
func (ToString) Arity() Arity   { return Arity{Min: 1, Max: 1} }
func (ToString) Signature() Signature {
	return Signature{Params: []Param{{Name: "value", Kind: KindAny}}, Returns: KindString,
		Category: CategoryText, Description: "Converts a value to text.", Example: "toString(value.id)"}
}

func (ToString) Call(args *Args) (any, error) {
	v, err := args.At(0)
	if err != nil || v == nil {
		return nil, err
	}
	return Text{}.Of(v), nil
}

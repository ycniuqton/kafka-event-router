package formula

import (
	"math"
	"time"

	"github.com/eventrouter/router/pkg/config"
	"github.com/eventrouter/router/pkg/engine/ctxhelpers"
	"github.com/eventrouter/router/pkg/engine/filter"
)

// MaxRoundDecimals bounds the decimals argument of round().
const MaxRoundDecimals = 15

// DecimalBase is the base of round() scaling.
const DecimalBase = 10

// Round rounds half away from zero; zero decimals give an integer.
type Round struct{}

func (Round) Name() FuncName { return FuncRound }
func (Round) Arity() Arity   { return Arity{Min: 1, Max: 2} }
func (Round) Signature() Signature {
	return Signature{Params: []Param{{Name: "number", Kind: KindNumber}, {Name: "decimals", Kind: KindInteger, Optional: true}},
		Returns: KindNumber, Category: CategoryNumber, Description: "Rounds to a number of decimals.", Example: "round(value.price * 1.1, 2)"}
}

func (Round) Call(args *Args) (any, error) {
	vals, err := args.All()
	if err != nil || vals[0] == nil {
		return nil, err
	}
	x, err := Numbers{}.Of(vals[0])
	if err != nil {
		return nil, err
	}
	decimals := 0.0
	if len(vals) > 1 {
		if decimals, err = (Numbers{}).Of(vals[1]); err != nil {
			return nil, err
		}
	}
	return Rounder{}.Round(x, int(decimals)), nil
}

// Rounder rounds numbers to decimals (shared by round() and the round recipe).
type Rounder struct{}

// Round returns an int64 for zero decimals, else a float64.
func (Rounder) Round(x float64, decimals int) any {
	d := min(max(decimals, 0), MaxRoundDecimals)
	scale := math.Pow(DecimalBase, float64(d))
	r := math.Round(x*scale) / scale
	return Numbers{}.Result(r, d == 0)
}

// ToNumber parses numbers from text; null stays null.
type ToNumber struct{}

func (ToNumber) Name() FuncName { return FuncToNumber }
func (ToNumber) Arity() Arity   { return Arity{Min: 1, Max: 1} }
func (ToNumber) Signature() Signature {
	return Signature{Params: []Param{{Name: "value", Kind: KindAny}}, Returns: KindNumber,
		Category: CategoryNumber, Description: "Converts text to a number.", Example: `toNumber(headers.retries)`}
}

func (ToNumber) Call(args *Args) (any, error) {
	v, err := args.At(0)
	if err != nil || v == nil {
		return nil, err
	}
	f, err := Numbers{}.Of(v)
	if err != nil {
		return nil, err
	}
	return Numbers{}.Result(f, true), nil
}

// Coalesce returns the first argument that is not null.
type Coalesce struct{}

func (Coalesce) Name() FuncName { return FuncCoalesce }
func (Coalesce) Arity() Arity   { return Arity{Min: 1, Max: ArityVariadic} }
func (Coalesce) Signature() Signature {
	return Signature{Params: []Param{{Name: "values", Kind: KindAny, Variadic: true}}, Returns: KindAny,
		Category: CategoryLogic, Description: "First value that is not null.", Example: `coalesce(value.nickname, value.name, "unknown")`}
}

func (Coalesce) Call(args *Args) (any, error) {
	for i := 0; i < args.Len(); i++ {
		v, err := args.At(i)
		if err != nil || v != nil {
			return v, err
		}
	}
	return nil, nil
}

// If evaluates only the selected branch.
type If struct{}

func (If) Name() FuncName { return FuncIf }
func (If) Arity() Arity   { return Arity{Min: 2, Max: 3} }
func (If) Signature() Signature {
	return Signature{Params: []Param{{Name: "condition", Kind: KindBoolean}, {Name: "then", Kind: KindAny}, {Name: "else", Kind: KindAny, Optional: true}},
		Returns: KindAny, Category: CategoryLogic, Description: "Chooses a value by condition.", Example: `if(value.amount > 100, "high", "low")`}
}

func (If) Call(args *Args) (any, error) {
	c, err := args.At(0)
	if err != nil {
		return nil, err
	}
	if (Truth{}).Of(c) {
		return args.At(1)
	}
	return args.At(2)
}

// Now returns the processing time frozen for the event (RFC 3339, UTC).
type Now struct{}

func (Now) Name() FuncName { return FuncNow }
func (Now) Arity() Arity   { return Arity{Min: 0, Max: 0} }
func (Now) Signature() Signature {
	return Signature{Params: []Param{}, Returns: KindString, Category: CategoryDate,
		Description: "Processing time of the event.", Example: "now()"}
}

func (Now) Call(args *Args) (any, error) {
	return args.Context().Now.UTC().Format(time.RFC3339Nano), nil
}

// FormatDate renders an RFC 3339 string or epoch millis in a layout and time zone.
type FormatDate struct{}

func (FormatDate) Name() FuncName { return FuncFormatDate }
func (FormatDate) Arity() Arity   { return Arity{Min: 2, Max: 3} }
func (FormatDate) Signature() Signature {
	return Signature{Params: []Param{{Name: "date", Kind: KindAny}, {Name: "layout", Kind: KindString}, {Name: "timezone", Kind: KindString, Optional: true}},
		Returns: KindString, Category: CategoryDate, Description: "Formats a date (layouts: rfc3339, rfc3339_ms, date, datetime).", Example: `formatDate(value.createdAt, "date", "Europe/Berlin")`}
}

func (FormatDate) Call(args *Args) (any, error) {
	vals, err := args.All()
	if err != nil || vals[0] == nil {
		return nil, err
	}
	t, ok := (filter.Coercer{}).Time(vals[0])
	if !ok {
		return nil, ErrNotDate
	}
	tz := ""
	if len(vals) > 2 && vals[2] != nil {
		tz = Text{}.Of(vals[2])
	}
	return args.Context().Helpers.Date.Format(t, ctxhelpers.DateLayout(Text{}.Of(vals[1])), tz)
}

// Mask applies a mask style (full, last4, email) with the shared masker.
type Mask struct{}

func (Mask) Name() FuncName { return FuncMask }
func (Mask) Arity() Arity   { return Arity{Min: 2, Max: 2} }
func (Mask) Signature() Signature {
	return Signature{Params: []Param{{Name: "value", Kind: KindAny}, {Name: "style", Kind: KindString}}, Returns: KindString,
		Category: CategoryPrivacy, Description: "Masks a value (styles: full, last4, email).", Example: `mask(value.card, "last4")`}
}

func (Mask) Call(args *Args) (any, error) {
	vals, err := args.All()
	if err != nil {
		return nil, err
	}
	return args.Context().Helpers.Mask.Mask(vals[0], config.MaskStyle(Text{}.Of(vals[1])))
}

// Hash returns the salted SHA-256 of a value with the shared hasher.
type Hash struct{}

func (Hash) Name() FuncName { return FuncHash }
func (Hash) Arity() Arity   { return Arity{Min: 1, Max: 1} }
func (Hash) Signature() Signature {
	return Signature{Params: []Param{{Name: "value", Kind: KindAny}}, Returns: KindString,
		Category: CategoryPrivacy, Description: "Salted SHA-256 hash (hex).", Example: "hash(value.email)"}
}

func (Hash) Call(args *Args) (any, error) {
	v, err := args.At(0)
	if err != nil {
		return nil, err
	}
	return args.Context().Helpers.Hash.Hash(args.Context().Ctx, v)
}

// Lookup reads a lookup table row by key; a missing row gives the default (or null).
type Lookup struct{}

func (Lookup) Name() FuncName { return FuncLookup }
func (Lookup) Arity() Arity   { return Arity{Min: 2, Max: 3} }
func (Lookup) Signature() Signature {
	return Signature{Params: []Param{{Name: "table", Kind: KindString}, {Name: "key", Kind: KindAny}, {Name: "default", Kind: KindAny, Optional: true}},
		Returns: KindAny, Category: CategoryLookup, Description: "Value of a lookup table row (table = lookup step id).", Example: `lookup("countries", value.cc, "unknown")`}
}

func (Lookup) Call(args *Args) (any, error) {
	vals, err := args.All()
	if err != nil {
		return nil, err
	}
	if v, ok := args.Context().Helpers.Lookup.Lookup(Text{}.Of(vals[0]), Text{}.Of(vals[1])); ok {
		return v, nil
	}
	if len(vals) > 2 {
		return vals[2], nil
	}
	return nil, nil
}

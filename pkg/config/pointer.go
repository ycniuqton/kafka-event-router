package config

import (
	"strconv"
	"strings"
)

// Pointer is an RFC 6901 JSON Pointer into a route document ("" is the root).
type Pointer string

// PointerRoot addresses the whole document.
const PointerRoot Pointer = ""

// PointerToken names well-known document members used to build pointers.
type PointerToken string

const (
	TokenBranches    PointerToken = "branches"
	TokenFilter      PointerToken = "filter"
	TokenTransform   PointerToken = "transform"
	TokenSteps       PointerToken = "steps"
	TokenDestination PointerToken = "destination"
	TokenSource      PointerToken = "source"
	TokenChildren    PointerToken = "children"
	TokenChild       PointerToken = "child"
	TokenCondition   PointerToken = "condition"
	TokenDelivery    PointerToken = "delivery"
	TokenMode        PointerToken = "mode"
	TokenConnection  PointerToken = "connectionId"
	TokenFormat      PointerToken = "format"
	TokenKeyFormat   PointerToken = "keyFormat"
	TokenAPIVersion  PointerToken = "apiVersion"
	TokenType        PointerToken = "type"
	TokenField       PointerToken = "field"
	TokenFields      PointerToken = "fields"
	TokenOp          PointerToken = "op"
	TokenCode        PointerToken = "code"
	TokenValue       PointerToken = "value"
	TokenValues      PointerToken = "values"
	TokenFrom        PointerToken = "from"
	TokenTo          PointerToken = "to"
	TokenAmount      PointerToken = "amount"
	TokenUnit        PointerToken = "unit"
	TokenKind        PointerToken = "kind"
	TokenPath        PointerToken = "path"
	TokenRef         PointerToken = "ref"
	TokenStyle       PointerToken = "style"
	TokenKey         PointerToken = "key"
	TokenName        PointerToken = "name"
	TokenTargetType  PointerToken = "targetType"
	TokenExpr        PointerToken = "expr"
	TokenID          PointerToken = "id"
	TokenSeparator   PointerToken = "separator"
	TokenPreset      PointerToken = "preset"
	TokenPattern     PointerToken = "pattern"
	TokenLayout      PointerToken = "layout"
	TokenTimezone    PointerToken = "timezone"
	TokenDecimals    PointerToken = "decimals"
	TokenCase        PointerToken = "case"
	TokenRows        PointerToken = "rows"
	TokenThenSteps   PointerToken = "thenSteps"
	TokenElseSteps   PointerToken = "elseSteps"
	TokenHeader      PointerToken = "header"
	TokenNewHeader   PointerToken = "newHeader"
	TokenStart       PointerToken = "start"
	TokenTarget      PointerToken = "target"
	TokenVersion     PointerToken = "version"
)

var pointerEscaper = strings.NewReplacer("~", "~0", "/", "~1")
var pointerUnescaper = strings.NewReplacer("~1", "/", "~0", "~")

// Key appends one object member name.
func (p Pointer) Key(name string) Pointer {
	return p + "/" + Pointer(pointerEscaper.Replace(name))
}

// Token appends a well-known member name.
func (p Pointer) Token(t PointerToken) Pointer {
	return p.Key(string(t))
}

// Index appends one array index.
func (p Pointer) Index(i int) Pointer {
	return p + "/" + Pointer(strconv.Itoa(i))
}

// Join appends a relative pointer (itself starting with "/" or empty).
func (p Pointer) Join(rel Pointer) Pointer {
	return p + rel
}

// Tokens splits the pointer into unescaped reference tokens.
func (p Pointer) Tokens() []string {
	if p == PointerRoot {
		return nil
	}
	parts := strings.Split(string(p)[1:], "/")
	for i, part := range parts {
		parts[i] = pointerUnescaper.Replace(part)
	}
	return parts
}

// PointerFromTokens builds a pointer from unescaped reference tokens.
func PointerFromTokens(tokens []string) Pointer {
	p := PointerRoot
	for _, t := range tokens {
		p = p.Key(t)
	}
	return p
}

// BranchPointer addresses branch i of the route.
func BranchPointer(i int) Pointer {
	return PointerRoot.Token(TokenBranches).Index(i)
}

// StepPointer addresses step j of branch i.
func StepPointer(branch, step int) Pointer {
	return BranchPointer(branch).Token(TokenTransform).Token(TokenSteps).Index(step)
}

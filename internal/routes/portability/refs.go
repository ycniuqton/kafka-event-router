package portability

import (
	"bytes"
	"encoding/json"
	"sort"

	"github.com/eventrouter/router/internal/routes/validation"
	"github.com/eventrouter/router/pkg/config"
)

// DocMember names route document members this package reads or rewrites.
type DocMember string

const (
	MemberAPIVersion   DocMember = "apiVersion"
	MemberName         DocMember = "name"
	MemberSource       DocMember = "source"
	MemberBranches     DocMember = "branches"
	MemberDestination  DocMember = "destination"
	MemberConnectionID DocMember = "connectionId"
	MemberErrorPolicy  DocMember = "errorPolicy"
	MemberDLQConn      DocMember = "dlqConnectionId"
	MemberDelivery     DocMember = "delivery"
	MemberLimits       DocMember = "limits"
	MemberGroup        DocMember = "consumerGroup"
)

// ConnectionSlot is one connection reference inside a decoded document tree.
type ConnectionSlot struct {
	Parent map[string]any
	Key    string
	Path   config.Pointer
}

// Value returns the referenced connection (id or name), "" when absent.
func (s ConnectionSlot) Value() string {
	v, _ := s.Parent[s.Key].(string)
	return v
}

// Set replaces the reference.
func (s ConnectionSlot) Set(v string) {
	s.Parent[s.Key] = v
}

// DocumentTree is a decoded route document whose connection references can be rewritten.
type DocumentTree struct {
	Root map[string]any
}

// ParseTree decodes raw keeping number precision; non-objects are rejected.
func ParseTree(raw []byte) (DocumentTree, error) {
	var root map[string]any
	if err := (config.JSONDecoder{}).Into(raw, &root); err != nil {
		return DocumentTree{}, err
	}
	if root == nil {
		root = map[string]any{}
	}
	return DocumentTree{Root: root}, nil
}

// Name returns the document's name member.
func (t DocumentTree) Name() string {
	n, _ := t.Root[string(MemberName)].(string)
	return n
}

// Slots lists source, branch destination and DLQ connection references (paths as in validation).
func (t DocumentTree) Slots() []ConnectionSlot {
	out := []ConnectionSlot{}
	if src, ok := t.Root[string(MemberSource)].(map[string]any); ok {
		out = append(out, ConnectionSlot{Parent: src, Key: string(MemberConnectionID), Path: config.PointerRoot.Token(config.TokenSource).Token(config.TokenConnection)})
	}
	branches, _ := t.Root[string(MemberBranches)].([]any)
	for i, b := range branches {
		branch, _ := b.(map[string]any)
		if dst, ok := branch[string(MemberDestination)].(map[string]any); ok {
			out = append(out, ConnectionSlot{Parent: dst, Key: string(MemberConnectionID), Path: config.BranchPointer(i).Token(config.TokenDestination).Token(config.TokenConnection)})
		}
	}
	if ep, ok := t.Root[string(MemberErrorPolicy)].(map[string]any); ok {
		out = append(out, ConnectionSlot{Parent: ep, Key: string(MemberDLQConn), Path: validation.DLQConnectionPointer})
	}
	return t.present(out)
}

// present keeps the slots that hold a non-empty reference.
func (DocumentTree) present(slots []ConnectionSlot) []ConnectionSlot {
	out := []ConnectionSlot{}
	for _, s := range slots {
		if s.Value() != "" {
			out = append(out, s)
		}
	}
	return out
}

// DropConsumerGroup removes source.consumerGroup when it equals group (the route's default), so an imported copy
// gets its own default group instead of sharing the exporter's.
func (t DocumentTree) DropConsumerGroup(group string) {
	src, ok := t.Root[string(MemberSource)].(map[string]any)
	if ok && src[string(MemberGroup)] == group {
		delete(src, string(MemberGroup))
	}
}

// Values returns the distinct references of slots.
func (t DocumentTree) Values() []string {
	seen := map[string]bool{}
	out := []string{}
	for _, s := range t.Slots() {
		if !seen[s.Value()] {
			seen[s.Value()] = true
			out = append(out, s.Value())
		}
	}
	return out
}

// JSON encodes the tree with the document members in reading order (apiVersion, name, source, …).
func (t DocumentTree) JSON() (json.RawMessage, error) {
	return DocumentOrder{}.Encode(t.Root)
}

// leadingMembers is the reading order of top-level members; others follow alphabetically.
var leadingMembers = []DocMember{MemberAPIVersion, MemberName, MemberSource, MemberBranches, MemberErrorPolicy, MemberDelivery, MemberLimits}

// DocumentOrder writes a top-level object with its members in reading order.
type DocumentOrder struct{}

// Encode renders root; nested objects keep encoding/json's sorted keys.
func (DocumentOrder) Encode(root map[string]any) (json.RawMessage, error) {
	keys := DocumentOrder{}.keys(root)
	var out bytes.Buffer
	out.WriteByte('{')
	for i, k := range keys {
		if i > 0 {
			out.WriteByte(',')
		}
		if err := (config.JSONText{}).Write(&out, k); err != nil {
			return nil, err
		}
		out.WriteByte(':')
		if err := (config.JSONText{}).Write(&out, root[k]); err != nil {
			return nil, err
		}
	}
	out.WriteByte('}')
	return out.Bytes(), nil
}

// keys returns the leading members present in root, then the rest sorted.
func (DocumentOrder) keys(root map[string]any) []string {
	used := map[string]bool{}
	keys := []string{}
	for _, m := range leadingMembers {
		if _, ok := root[string(m)]; ok {
			keys = append(keys, string(m))
			used[string(m)] = true
		}
	}
	rest := []string{}
	for k := range root {
		if !used[k] {
			rest = append(rest, k)
		}
	}
	sort.Strings(rest)
	return append(keys, rest...)
}

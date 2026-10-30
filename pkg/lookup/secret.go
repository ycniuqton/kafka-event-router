package lookup

import "encoding/json"

// SecretField names one write-only secret of a source.
type SecretField string

const (
	SecretToken       SecretField = "token"
	SecretPassword    SecretField = "password"
	SecretHeaderValue SecretField = "headerValue"
)

// SecretFields lists every field in stable order.
var SecretFields = []SecretField{SecretToken, SecretPassword, SecretHeaderValue}

// SecretBundle holds the decrypted secrets of a source (never logged, never returned by the public API).
type SecretBundle struct {
	Token       string `json:"token,omitempty"`
	Password    string `json:"password,omitempty"`
	HeaderValue string `json:"headerValue,omitempty"`
}

// fieldRefs returns pointers to every member keyed by field.
func (b *SecretBundle) fieldRefs() map[SecretField]*string {
	return map[SecretField]*string{SecretToken: &b.Token, SecretPassword: &b.Password, SecretHeaderValue: &b.HeaderValue}
}

// Get returns the value of f.
func (b SecretBundle) Get(f SecretField) string { return *b.fieldRefs()[f] }

// Has reports whether f is set.
func (b SecretBundle) Has(f SecretField) bool { return b.Get(f) != "" }

// IsEmpty reports whether no secret is set.
func (b SecretBundle) IsEmpty() bool { return b == SecretBundle{} }

// Merge overlays the set members of patch.
func (b SecretBundle) Merge(patch SecretBundle) SecretBundle {
	out := b
	refs := out.fieldRefs()
	for _, f := range SecretFields {
		if patch.Has(f) {
			*refs[f] = patch.Get(f)
		}
	}
	return out
}

// Only keeps the given fields.
func (b SecretBundle) Only(fields []SecretField) SecretBundle {
	var out SecretBundle
	refs := out.fieldRefs()
	for _, f := range fields {
		*refs[f] = b.Get(f)
	}
	return out
}

// Set lists the set fields in stable order.
func (b SecretBundle) Set() []SecretField {
	var out []SecretField
	for _, f := range SecretFields {
		if b.Has(f) {
			out = append(out, f)
		}
	}
	return out
}

// SecretCodec encodes bundles for the secrets Manager.
type SecretCodec struct{}

// Encode returns the JSON form.
func (SecretCodec) Encode(b SecretBundle) ([]byte, error) { return json.Marshal(b) }

// Decode parses the JSON form.
func (SecretCodec) Decode(raw []byte) (SecretBundle, error) {
	var b SecretBundle
	err := json.Unmarshal(raw, &b)
	return b, err
}

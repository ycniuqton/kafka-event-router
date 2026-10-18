package host

// MaskFn is ctx.mask.email / last4 / full, delegating to the shared Masker.
type MaskFn struct {
	name     FnName
	style    MaskStyle
	services HostServices
}

func (f MaskFn) Name() FnName { return f.name }

func (f MaskFn) Doc() HelperDoc {
	return HelperDoc{Name: f.name, Signature: "(value: unknown) => string", Doc: maskDocs[f.style]}
}

// Invoke masks the first argument.
func (f MaskFn) Invoke(call *Call) (any, error) {
	v, err := ArgsOf(f.name, call).Value(0)
	if err != nil {
		return nil, err
	}
	return f.services.Masker().Mask(v, f.style)
}

var maskDocs = map[MaskStyle]string{
	MaskEmail: "Masks an email but its first letter and the domain, e.g. j***@x.com.",
	MaskLast4: "Masks everything except the last 4 characters.",
	MaskFull:  "Replaces every character with a mask symbol.",
}

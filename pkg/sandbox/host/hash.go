package host

// HashFn is ctx.hash: salted hash with the team salt bound at Load.
type HashFn struct {
	services HostServices
}

func (HashFn) Name() FnName { return FnHash }

func (HashFn) Doc() HelperDoc {
	return HelperDoc{Name: FnHash, Signature: "(value: unknown) => string",
		Doc: "Stable salted SHA-256 hash (hex); the salt is per team and never visible to code."}
}

// Invoke hashes the first argument.
func (f HashFn) Invoke(call *Call) (any, error) {
	v, err := ArgsOf(FnHash, call).Value(0)
	if err != nil {
		return nil, err
	}
	return f.services.Hasher().Hash(call.Ctx, v)
}

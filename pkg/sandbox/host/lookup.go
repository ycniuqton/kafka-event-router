package host

import "fmt"

// LookupFn is ctx.lookup(table, key): the route version's pinned library tables first, then the
// engine's tables; a missing table or key yields null, oversized arguments throw in the guest.
type LookupFn struct {
	services HostServices
}

func (LookupFn) Name() FnName { return FnLookup }

func (LookupFn) Doc() HelperDoc {
	return HelperDoc{Name: FnLookup, Signature: "(table: string, key: unknown) => unknown",
		Doc: "Looks up a key in a lookup table of this route (library tables at the version pinned by the route); returns null when the table or key is missing."}
}

// Invoke resolves the key's text form in the table.
func (f LookupFn) Invoke(call *Call) (any, error) {
	args := ArgsOf(FnLookup, call)
	if err := args.Require(2); err != nil {
		return nil, err
	}
	table, err := args.String(0)
	if err != nil {
		return nil, err
	}
	key, err := args.Value(1)
	if err != nil {
		return nil, err
	}
	text := fmt.Sprint(key)
	if err := (lookupArgs{table: table, key: text}).check(); err != nil {
		return nil, err
	}
	v, _ := f.services.Lookups().Lookup(table, text)
	return v, nil
}

// lookupArgs are the decoded ctx.lookup arguments.
type lookupArgs struct {
	table, key string
}

// check enforces the argument size limits.
func (a lookupArgs) check() error {
	if n := len(a.table); n > int(LookupMaxTableBytes) {
		return fmt.Errorf(string(MsgArgTooLong), FnLookup, ArgTable, n, LookupMaxTableBytes)
	}
	if n := len(a.key); n > int(LookupMaxKeyBytes) {
		return fmt.Errorf(string(MsgArgTooLong), FnLookup, ArgKey, n, LookupMaxKeyBytes)
	}
	return nil
}

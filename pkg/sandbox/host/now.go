package host

// NowFn is ctx.now: the processing time frozen for the current event.
type NowFn struct{}

func (NowFn) Name() FnName { return FnNow }

func (NowFn) Doc() HelperDoc {
	return HelperDoc{Name: FnNow, Signature: "() => string",
		Doc: "Processing time of this event as an ISO-8601 UTC string; frozen for the whole call."}
}

// Invoke returns the frozen clock of the call.
func (NowFn) Invoke(call *Call) (any, error) {
	return TimeValue{}.ISO(call.Now), nil
}

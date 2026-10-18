package host

// FormatDateFn is ctx.formatDate(value, layout, tz) using the shared DateFormatter.
type FormatDateFn struct {
	services HostServices
}

func (FormatDateFn) Name() FnName { return FnFormatDate }

func (FormatDateFn) Doc() HelperDoc {
	return HelperDoc{Name: FnFormatDate, Signature: "(value: string | number, layout: string, tz?: string) => string",
		Doc: "Formats an ISO string or epoch milliseconds in a named layout (rfc3339, rfc3339_ms, date, datetime) and IANA zone."}
}

// Invoke parses the value, then formats it.
func (f FormatDateFn) Invoke(call *Call) (any, error) {
	args := ArgsOf(FnFormatDate, call)
	if err := args.Require(2); err != nil {
		return nil, err
	}
	t, err := args.Time(0)
	if err != nil {
		return nil, err
	}
	layout, err := args.String(1)
	if err != nil {
		return nil, err
	}
	tz, err := args.String(2)
	if err != nil {
		return nil, err
	}
	return f.services.DateFormatter().Format(t, layout, tz)
}

package host

// FnName is the name of a ctx helper as seen by user code.
type FnName string

const (
	FnMaskEmail  FnName = "mask.email"
	FnMaskLast4  FnName = "mask.last4"
	FnMaskFull   FnName = "mask.full"
	FnHash       FnName = "hash"
	FnNow        FnName = "now"
	FnFormatDate FnName = "formatDate"
	FnLookup     FnName = "lookup"
	FnLog        FnName = "log"
)

// MaskStyle names a masking strategy shared with the mask recipe.
type MaskStyle string

const (
	MaskEmail MaskStyle = "email"
	MaskLast4 MaskStyle = "last4"
	MaskFull  MaskStyle = "full"
)

// Message is a user-facing host error template.
type Message string

const (
	MsgUnknownHelper Message = "unknown ctx helper %q"
	MsgBadArguments  Message = "%s: invalid arguments: %v"
	MsgArgCount      Message = "%s expects %d argument(s), got %d"
	MsgNotString     Message = "%s: argument %d must be a string"
	MsgArgTooLong    Message = "%s: %s is %d bytes; the limit is %d bytes"
	MsgPinConflict   Message = "lookup table %q is pinned to versions %d and %d"
	MsgPinMissing    Message = "lookup table %q version %d: %v"
)

// ArgName names a helper argument in host error messages.
type ArgName string

const (
	ArgTable ArgName = "table name"
	ArgKey   ArgName = "key"
)

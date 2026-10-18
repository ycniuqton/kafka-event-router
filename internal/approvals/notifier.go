package approvals

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
)

// NotifierKind selects the notification transport (ROUTER_APPROVALS_NOTIFIER).
type NotifierKind string

const (
	NotifierLog    NotifierKind = "log"
	NotifierMemory NotifierKind = "memory"
)

// Notifier is the base of the notification domain: monitoring's WS /events bus implements it (bell entries).
type Notifier interface {
	Kind() NotifierKind
	Publish(ctx context.Context, e ApprovalEvent) error
}

// NotifierFactory builds one notifier.
type NotifierFactory func() (Notifier, error)

// NotifierRegistry maps kinds to factories (config-selected).
type NotifierRegistry struct {
	factories map[NotifierKind]NotifierFactory
}

// NewNotifierRegistry registers the built-in log and memory notifiers.
func NewNotifierRegistry(log *slog.Logger) *NotifierRegistry {
	r := &NotifierRegistry{factories: map[NotifierKind]NotifierFactory{}}
	r.Register(NotifierLog, func() (Notifier, error) { return &LogNotifier{log: log}, nil })
	r.Register(NotifierMemory, func() (Notifier, error) { return &MemoryNotifier{}, nil })
	return r
}

// Register adds or replaces a factory (monitoring registers its bus notifier at wiring time).
func (r *NotifierRegistry) Register(kind NotifierKind, f NotifierFactory) {
	r.factories[kind] = f
}

// Build instantiates the configured notifier.
func (r *NotifierRegistry) Build(kind NotifierKind) (Notifier, error) {
	f, ok := r.factories[kind]
	if !ok {
		return nil, fmt.Errorf("approvals: no notifier registered for %q", kind)
	}
	return f()
}

// NotifierLogMessage is the log line of a published approval event.
type NotifierLogMessage string

const LogApprovalEvent NotifierLogMessage = "approval event"

// LogAttr names log attributes of approval events.
type LogAttr string

const (
	AttrKind       LogAttr = "kind"
	AttrApproval   LogAttr = "approval_id"
	AttrRoute      LogAttr = "route"
	AttrRecipients LogAttr = "recipients"
)

// LogNotifier writes events to the structured log (default until the WS bus lands).
type LogNotifier struct {
	log *slog.Logger
}

// Kind implements Notifier.
func (*LogNotifier) Kind() NotifierKind { return NotifierLog }

// Publish implements Notifier.
func (n *LogNotifier) Publish(_ context.Context, e ApprovalEvent) error {
	n.log.Info(string(LogApprovalEvent), string(AttrKind), string(e.Kind), string(AttrApproval), e.ApprovalID.String(),
		string(AttrRoute), e.RouteName, string(AttrRecipients), len(e.Recipients))
	return nil
}

// MemoryNotifier keeps events in memory (tests, single-process demos).
type MemoryNotifier struct {
	mu     sync.Mutex
	events []ApprovalEvent
}

// Kind implements Notifier.
func (*MemoryNotifier) Kind() NotifierKind { return NotifierMemory }

// Publish implements Notifier.
func (n *MemoryNotifier) Publish(_ context.Context, e ApprovalEvent) error {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.events = append(n.events, e)
	return nil
}

// Events returns a copy of the published events.
func (n *MemoryNotifier) Events() []ApprovalEvent {
	n.mu.Lock()
	defer n.mu.Unlock()
	return append([]ApprovalEvent(nil), n.events...)
}

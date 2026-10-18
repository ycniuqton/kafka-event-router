package cpclient

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"strings"

	"github.com/eventrouter/router/internal/runtime/domain"
)

// SSEField names the server-sent-events line prefixes this stream uses.
type SSEField string

const SSEData SSEField = "data:"

// EventStream decodes "data: {json}" server-sent events into change hints (comments are keep-alives).
type EventStream struct{}

// Read forwards hints until the body ends or ctx is cancelled.
func (EventStream) Read(ctx context.Context, body io.Reader, out chan<- domain.ChangeHint) {
	scanner := bufio.NewScanner(body)
	for scanner.Scan() {
		payload, ok := strings.CutPrefix(scanner.Text(), string(SSEData))
		if !ok {
			continue
		}
		var hint domain.ChangeHint
		if json.Unmarshal([]byte(strings.TrimSpace(payload)), &hint) != nil {
			continue
		}
		select {
		case out <- hint:
		case <-ctx.Done():
			return
		}
	}
}

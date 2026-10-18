package quickjs

import (
	"encoding/json"

	"github.com/eventrouter/router/pkg/sandbox"
)

// guestError is the error object produced by the prelude's describe().
type guestError struct {
	Kind    sandbox.ErrorKind `json:"kind"`
	Message string            `json:"message"`
	Stack   string            `json:"stack"`
}

// toError converts a guest error, mapping its stack to a user-code position.
func (g guestError) toError(c sandbox.ErrorClassifier, pos stackPosition) *sandbox.Error {
	e := c.New(g.Kind, g.Message)
	e.Stack = g.Stack
	e.Line, e.Column = pos.find(g.Stack)
	return e
}

// guestEnvelope is what invoke() returns.
type guestEnvelope struct {
	OK     bool            `json:"ok"`
	Output json.RawMessage `json:"output"`
	Bool   bool            `json:"bool"`
	Error  *guestError     `json:"error"`
}

// envelopeOverhead bounds the bytes invoke() adds around the user output.
type envelopeOverhead int64

const envelopeBytes envelopeOverhead = 64

// replyDecoder validates an envelope against the limits.
type replyDecoder struct {
	classifier sandbox.ErrorClassifier
	limits     sandbox.Limits
	position   stackPosition
}

// decode returns the reply or a typed error.
func (d replyDecoder) decode(raw []byte) (sandbox.GuestReply, error) {
	var env guestEnvelope
	if err := json.Unmarshal(raw, &env); err != nil {
		return sandbox.GuestReply{}, d.classifier.New(sandbox.ErrorInternal, err.Error())
	}
	if !env.OK {
		return sandbox.GuestReply{}, d.failure(env.Error)
	}
	if size := int64(len(env.Output)); size > d.limits.MaxOutputBytes {
		return sandbox.GuestReply{}, d.classifier.New(sandbox.ErrorOutputTooLarge,
			sandbox.MsgOutputTooLarge.Format(size, d.limits.MaxOutputBytes))
	}
	return sandbox.GuestReply{Output: env.Output, Bool: env.Bool}, nil
}

func (d replyDecoder) failure(g *guestError) error {
	if g == nil {
		return d.classifier.New(sandbox.ErrorInternal, string(sandbox.MsgInvalidOutput))
	}
	return g.toError(d.classifier, d.position)
}

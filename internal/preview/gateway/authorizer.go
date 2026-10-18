package gateway

import (
	"context"
	"errors"

	"github.com/eventrouter/router/internal/auth"
	"github.com/eventrouter/router/internal/platform/problem"
	"github.com/eventrouter/router/internal/preview"
	"github.com/eventrouter/router/internal/rbac"
)

// Permissions used by the preview gateway (keys from access-control's catalog).
var (
	PermSampleRead  = auth.Permission{Key: string(rbac.PermSampleRead), Access: auth.AccessRead}
	PermUpload      = auth.Permission{Key: string(rbac.PermRouteWrite), Access: auth.AccessWrite}
	PermTestRun     = auth.Permission{Key: string(rbac.PermRouteWrite), Access: auth.AccessWrite}
	PermPreviewRead = auth.Permission{Key: string(rbac.PermRouteRead), Access: auth.AccessRead}
	PermTestTopic   = auth.Permission{Key: string(rbac.PermTopicCreate), Access: auth.AccessWrite}
)

// TestTopicField is the request member a denied test-topic write points at.
const TestTopicField = "/writeToTopic"

// DetailTestTopicForbidden is shown when the caller may not create topics on the destination connection.
const DetailTestTopicForbidden = "You can't create a test topic on this destination connection. Ask an admin for topic.create, or run the test without writing to a topic."

// ErrTestTopicForbidden is the 403 of a test-topic write without topic.create.
var ErrTestTopicForbidden = problem.NewError(problem.KindForbidden, DetailTestTopicForbidden)

// ErrConnectionForbidden uses the standard forbidden kind: the API catalog renders gateway errors.
var ErrConnectionForbidden = problem.NewError(problem.KindForbidden, string(preview.DetailForbidden))

// ObjectConnection is the object type of connection checks.
const ObjectConnection = string(rbac.ResourceConnection)

// PreviewAuthorizer decides whether the caller may load samples and builds the caller directive;
// Samples (access-control SampleVisibility) decides masking: Viewers always, others on sensitive connections.
type PreviewAuthorizer struct {
	Authz   auth.Authorizer
	Samples rbac.SampleAccess
}

// CheckLoad: uploads need editor rights (connection optional); demo samples without a connection need
// route read (synthetic data); every other load needs sample.read on the connection.
func (a PreviewAuthorizer) CheckLoad(ctx context.Context, p auth.Principal, req preview.LoadRequest) error {
	if req.Mode == preview.ModeUpload {
		return a.deny(a.Authz.Authorize(ctx, p, PermUpload, auth.ObjectRef{}), "")
	}
	if req.Mode == preview.ModeDemo && req.ConnectionID == "" {
		return a.deny(a.Authz.Authorize(ctx, p, PermPreviewRead, auth.ObjectRef{}), "")
	}
	obj := auth.ObjectRef{Type: ObjectConnection, ID: req.ConnectionID}
	return a.deny(a.Authz.Authorize(ctx, p, PermSampleRead, obj), string(preview.FieldConnectionID))
}

// CheckTestTopic: writing a test topic needs topic.create on the destination connection.
func (a PreviewAuthorizer) CheckTestTopic(ctx context.Context, p auth.Principal, connectionID string) error {
	obj := auth.ObjectRef{Type: ObjectConnection, ID: connectionID}
	err := a.Authz.Authorize(ctx, p, PermTestTopic, obj)
	var classified problem.Classified
	if err != nil && errors.As(err, &classified) && classified.ProblemKind() == problem.KindForbidden {
		return ErrTestTopicForbidden.At(TestTopicField).Wrap(err)
	}
	return err
}

// deny maps a forbidden decision to the preview problem pointing at path.
func (PreviewAuthorizer) deny(err error, path string) error {
	var classified problem.Classified
	if err == nil || !errors.As(err, &classified) || classified.ProblemKind() != problem.KindForbidden {
		return err
	}
	return ErrConnectionForbidden.At(path).Wrap(err)
}

// Caller builds the directive for p reading samples of connectionID ("" = no connection: the role alone
// decides). An unknown connection is a 404 at /connectionId; any other visibility failure fails closed (masked).
func (a PreviewAuthorizer) Caller(ctx context.Context, p auth.Principal, connectionID string) (preview.Caller, error) {
	c := a.Identity(p)
	if a.Samples == nil {
		return c, nil
	}
	decision, err := a.Samples.Decide(ctx, rbac.SurfacePreview, connectionID)
	if errors.Is(err, rbac.ErrUnknownConnection) {
		return c, preview.ErrConnectionGone.At(string(preview.FieldConnectionID)).Wrap(err)
	}
	c.Mask = decision.Masked || err != nil
	return c, nil
}

// Identity is the unmasked directive naming p (session lookups, which carry no payload).
func (PreviewAuthorizer) Identity(p auth.Principal) preview.Caller {
	c := preview.Caller{UserID: p.UserID.String(), MaskStyle: preview.MaskTypePreserving}
	if len(p.Teams) > 0 {
		c.TeamID = p.Teams[0].TeamID.String()
	}
	return c
}

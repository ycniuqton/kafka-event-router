package httpapi

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/eventrouter/router/internal/dlq"
	"github.com/eventrouter/router/internal/dlq/jobs"
	"github.com/eventrouter/router/internal/platform/api"
)

// EditValueBody is an edited key or value as sent: data is kept raw so JSON numbers keep their precision.
type EditValueBody struct {
	Encoding dlq.Encoding    `json:"encoding"`
	Data     json.RawMessage `json:"data"`
}

// EditBody is the edit of POST …/{dlqId}/replay (omitted member = keep the original).
type EditBody struct {
	Key     *EditValueBody       `json:"key"`
	Value   *EditValueBody       `json:"value"`
	Headers *[]dlq.EncodedHeader `json:"headers"`
}

// ReplayBody is the body of POST …/{dlqId}/replay.
type ReplayBody struct {
	Edit   *EditBody `json:"edit"`
	DryRun bool      `json:"dryRun"`
	Note   string    `json:"note"`
}

// EditOf converts edit bodies into domain edits.
type EditOf struct{}

// Edit returns the domain edit of b (nil keeps the record unchanged).
func (EditOf) Edit(b *EditBody) (*dlq.RecordEdit, error) {
	if b == nil {
		return nil, nil
	}
	key, err := EditOf{}.value(b.Key, dlq.PathEditKey)
	if err != nil {
		return nil, err
	}
	value, err := EditOf{}.value(b.Value, dlq.PathEditValue)
	if err != nil {
		return nil, err
	}
	return &dlq.RecordEdit{Key: key, Value: value, Headers: b.Headers}, nil
}

// value keeps JSON data raw and unwraps the string of text encodings.
func (EditOf) value(v *EditValueBody, path dlq.RequestPath) (*dlq.EncodedValue, error) {
	if v == nil {
		return nil, nil
	}
	if v.Encoding == dlq.EncodingJSON {
		var data any = v.Data
		if len(v.Data) == 0 || string(v.Data) == "null" {
			data = nil
		}
		return &dlq.EncodedValue{Encoding: v.Encoding, Data: data}, nil
	}
	var text string
	if err := json.Unmarshal(v.Data, &text); err != nil {
		return nil, dlq.ErrInvalidEdit.At(string(path))
	}
	return &dlq.EncodedValue{Encoding: v.Encoding, Data: text}, nil
}

// ReplayDTO answers POST …/{dlqId}/replay (design 00001 §10).
type ReplayDTO struct {
	DryRun          bool              `json:"dryRun"`
	Status          dlq.ReplayStatus  `json:"status,omitempty"`
	ActiveVersionNo int               `json:"activeVersionNo"`
	Masked          bool              `json:"masked"`
	Diff            []dlq.DiffEntry   `json:"diff"`
	Result          *dlq.DryRunResult `json:"result,omitempty"`
	Report          *RetryDTO         `json:"report,omitempty"`
	Job             *jobs.Job         `json:"job,omitempty"`
}

// ReplayHandler serves POST /routes/{id}/dlq/{dlqId}/replay: dry-run or replay one (edited) dead letter.
type ReplayHandler struct{ d Deps }

func (h *ReplayHandler) Route() api.RouteSpec {
	return api.RouteSpec{Method: http.MethodPost, Pattern: string(PathReplay), Permission: PermAct, Object: RouteObject,
		Mutating: true, AuditAction: dlq.ActionDLQEditedReplayed, BodyLimit: api.BodyLimitLarge}
}

func (h *ReplayHandler) Serve(w http.ResponseWriter, r *http.Request) error {
	body, err := api.Decoder[ReplayBody]{Limit: api.BodyLimitLarge}.Decode(w, r)
	if err != nil {
		return err
	}
	edit, err := EditOf{}.Edit(body.Edit)
	if err != nil {
		return err
	}
	req := dlq.ReplayRequest{Edit: edit, DryRun: body.DryRun, Note: body.Note}
	res, err := h.d.Manager.Replay(r.Context(), Caller{}.Viewer(r), Caller{}.Actor(r), chi.URLParam(r, string(ParamRouteID)), chi.URLParam(r, string(ParamDLQID)), req)
	if err != nil {
		return err
	}
	return ReplayResponse{d: h.d}.Write(w, r, res)
}

// ReplayResponse renders replay results; a pending replay is stored as a job first.
type ReplayResponse struct{ d Deps }

// Write answers 200 (dry run, done) or 202 (pending approval, with the job).
func (p ReplayResponse) Write(w http.ResponseWriter, r *http.Request, res dlq.ReplayResult) error {
	out := ReplayDTO{DryRun: res.DryRun, Status: res.Status, ActiveVersionNo: res.ActiveVersionNo, Masked: res.Masked, Diff: res.Diff, Result: res.Dry}
	if res.Report != nil {
		report := ReportDTOs{}.Retry(*res.Report)
		out.Report = &report
	}
	if res.Pending == nil {
		return p.d.Responder.JSON(w, http.StatusOK, out)
	}
	svc, err := JobService(p).Get()
	if err != nil {
		return err
	}
	job, err := svc.CreatePending(r.Context(), Caller{}.Actor(r), *res.Pending)
	if err != nil {
		return err
	}
	job = CallerJobs{}.One(r, job)
	out.Job = &job
	return p.d.Responder.JSON(w, http.StatusAccepted, out)
}

// DryRunHandler serves POST /routes/{id}/dlq/dry-run: a selection or group sample through the active version.
type DryRunHandler struct{ d Deps }

func (h *DryRunHandler) Route() api.RouteSpec {
	return api.RouteSpec{Method: http.MethodPost, Pattern: string(PathDryRun), Permission: PermAct, Object: RouteObject,
		Mutating: true, AuditAction: dlq.ActionDLQReplayPreview}
}

func (h *DryRunHandler) Serve(w http.ResponseWriter, r *http.Request) error {
	body, err := api.Decoder[SelectionBody]{}.Decode(w, r)
	if err != nil {
		return err
	}
	sel, err := RequestValidator{}.Selection(body)
	if err != nil {
		return err
	}
	report, err := h.d.Manager.DryRun(r.Context(), Caller{}.Viewer(r), chi.URLParam(r, string(ParamRouteID)), sel)
	if err != nil {
		return err
	}
	return h.d.Responder.JSON(w, http.StatusOK, report)
}

package query

import (
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"time"
)

// PromStatus is the status member of a Prometheus API answer.
type PromStatus string

const PromSuccess PromStatus = "success"

// PromResultType is the data.resultType member.
type PromResultType string

const (
	ResultVector PromResultType = "vector"
	ResultMatrix PromResultType = "matrix"
	ResultScalar PromResultType = "scalar"
)

// promEnvelope is the JSON shape of every Prometheus API answer.
type promEnvelope struct {
	Status    PromStatus `json:"status"`
	ErrorType string     `json:"errorType"`
	Error     string     `json:"error"`
	Data      struct {
		ResultType PromResultType  `json:"resultType"`
		Result     json.RawMessage `json:"result"`
	} `json:"data"`
}

type promSample struct {
	Metric Labels             `json:"metric"`
	Value  [2]json.RawMessage `json:"value"`
}

type promSeries struct {
	Metric Labels               `json:"metric"`
	Values [][2]json.RawMessage `json:"values"`
}

// PromResponseDecoder turns API answers into vectors and series; malformed answers are unavailable errors.
type PromResponseDecoder struct{}

// Vector decodes an instant-query answer (vector or scalar).
func (d PromResponseDecoder) Vector(body []byte) (Vector, error) {
	env, err := d.envelope(body)
	if err != nil {
		return nil, err
	}
	if env.Data.ResultType == ResultScalar {
		var pair [2]json.RawMessage
		if err := json.Unmarshal(env.Data.Result, &pair); err != nil {
			return nil, Unavailable(err)
		}
		p, err := d.point(pair)
		return Vector{{Labels: Labels{}, Value: p.Value}}, err
	}
	var raw []promSample
	if err := json.Unmarshal(env.Data.Result, &raw); err != nil {
		return nil, Unavailable(err)
	}
	out := make(Vector, 0, len(raw))
	for _, s := range raw {
		p, err := d.point(s.Value)
		if err != nil {
			return nil, err
		}
		out = append(out, Sample{Labels: s.Metric, Value: p.Value})
	}
	return out, nil
}

// Matrix decodes a range-query answer.
func (d PromResponseDecoder) Matrix(body []byte) ([]Series, error) {
	env, err := d.envelope(body)
	if err != nil {
		return nil, err
	}
	var raw []promSeries
	if err := json.Unmarshal(env.Data.Result, &raw); err != nil {
		return nil, Unavailable(err)
	}
	out := make([]Series, 0, len(raw))
	for _, s := range raw {
		points := make([]Point, 0, len(s.Values))
		for _, pair := range s.Values {
			p, err := d.point(pair)
			if err != nil {
				return nil, err
			}
			points = append(points, p)
		}
		out = append(out, Series{Labels: s.Metric, Points: points})
	}
	return out, nil
}

func (PromResponseDecoder) envelope(body []byte) (promEnvelope, error) {
	var env promEnvelope
	if err := json.Unmarshal(body, &env); err != nil {
		return env, Unavailable(fmt.Errorf("malformed answer: %w", err))
	}
	if env.Status != PromSuccess {
		return env, Unavailable(fmt.Errorf("%s: %s", env.ErrorType, env.Error))
	}
	return env, nil
}

// point decodes [unixSeconds, "value"]; NaN and ±Inf decode to 0 so JSON stays valid.
func (PromResponseDecoder) point(pair [2]json.RawMessage) (Point, error) {
	var at float64
	var raw string
	if err := json.Unmarshal(pair[0], &at); err != nil {
		return Point{}, Unavailable(err)
	}
	if err := json.Unmarshal(pair[1], &raw); err != nil {
		return Point{}, Unavailable(err)
	}
	v, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return Point{}, Unavailable(err)
	}
	if math.IsNaN(v) || math.IsInf(v, 0) {
		v = 0
	}
	return Point{At: time.Unix(int64(at), 0).UTC(), Value: v}, nil
}

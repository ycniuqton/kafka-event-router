package sampling

import (
	"bufio"
	"bytes"
	"strconv"
	"strings"
)

// Series is one parsed Prometheus sample.
type Series struct {
	Name   string
	Labels map[string]string
	Value  float64
}

// PromText parses the Prometheus text exposition format (enough for counters/gauges; histograms appear as their
// _bucket/_sum/_count series).
type PromText struct{}

// Parse returns every sample line.
func (PromText) Parse(raw []byte) []Series {
	var out []Series
	sc := bufio.NewScanner(bytes.NewReader(raw))
	sc.Buffer(make([]byte, 64*1024), 4*1024*1024)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || line[0] == '#' {
			continue
		}
		if s, ok := (PromText{}).line(line); ok {
			out = append(out, s)
		}
	}
	return out
}

func (PromText) line(line string) (Series, bool) {
	s := Series{Labels: map[string]string{}}
	rest := line
	if i := strings.IndexByte(line, '{'); i >= 0 {
		j := strings.LastIndexByte(line, '}')
		if j < i {
			return s, false
		}
		s.Name = line[:i]
		s.Labels = PromText{}.labels(line[i+1 : j])
		rest = strings.TrimSpace(line[j+1:])
	} else {
		name, value, ok := strings.Cut(line, " ")
		if !ok {
			return s, false
		}
		s.Name, rest = name, strings.TrimSpace(value)
	}
	v, err := strconv.ParseFloat(strings.Fields(rest)[0], 64)
	s.Value = v
	return s, err == nil
}

func (PromText) labels(body string) map[string]string {
	out := map[string]string{}
	for body != "" {
		name, after, ok := strings.Cut(body, `="`)
		if !ok {
			break
		}
		var val strings.Builder
		i := 0
		for ; i < len(after) && after[i] != '"'; i++ {
			if after[i] == '\\' && i+1 < len(after) {
				i++
			}
			val.WriteByte(after[i])
		}
		out[strings.TrimLeft(name, ", ")] = val.String()
		body = strings.TrimLeft(after[min(i+1, len(after)):], ",")
	}
	return out
}

// Select sums the values of series named name whose labels match every given pair (label value "" = any; a
// route_id filter set restricts to the stress run's routes).
type Select struct {
	Name   string
	Label  string
	Values map[string]bool
}

// Sum returns the sum over matching series and whether any matched.
func (sel Select) Sum(series []Series) (float64, bool) {
	var total float64
	found := false
	for _, s := range series {
		if s.Name != sel.Name {
			continue
		}
		if sel.Label != "" && !sel.Values[s.Labels[sel.Label]] {
			continue
		}
		total += s.Value
		found = true
	}
	return total, found
}

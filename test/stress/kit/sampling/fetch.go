// Package sampling collects the runner's 5 s observations: worker Prometheus metrics, consumer-group lag and /proc
// statistics of named processes, from local or remote (ssh-prefixed) sources.
package sampling

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/eventrouter/router/test/stress/kit/scenario"
)

// Timeouts of one fetch.
var Timeouts = struct {
	Fetch time.Duration
}{Fetch: 4 * time.Second}

// Fetcher obtains the bytes of one source.
type Fetcher interface {
	Fetch(ctx context.Context) ([]byte, error)
}

// httpFetcher GETs a URL.
type httpFetcher struct {
	url string
}

// Fetch performs the GET.
func (f httpFetcher) Fetch(ctx context.Context) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, Timeouts.Fetch)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, f.url, nil)
	if err != nil {
		return nil, err
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GET %s: HTTP %d", f.url, res.StatusCode)
	}
	return io.ReadAll(res.Body)
}

// commandFetcher runs a shell command (e.g. through ssh) and returns its stdout.
type commandFetcher struct {
	command string
}

// Fetch runs the command.
func (f commandFetcher) Fetch(ctx context.Context) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, Timeouts.Fetch)
	defer cancel()
	out, err := exec.CommandContext(ctx, "sh", "-c", f.command).Output()
	if err != nil {
		return nil, fmt.Errorf("command %q: %w", f.command, err)
	}
	return out, nil
}

// fileFetcher reads a local file.
type fileFetcher struct {
	path string
}

// Fetch reads the file.
func (f fileFetcher) Fetch(context.Context) ([]byte, error) {
	return os.ReadFile(f.path)
}

// FetcherManager builds fetchers by configured kind.
type FetcherManager struct{}

var fetcherFactories = map[scenario.SourceKind]func(scenario.SourceSpec) Fetcher{
	scenario.SourceHTTP:    func(s scenario.SourceSpec) Fetcher { return httpFetcher{url: s.URL} },
	scenario.SourceCommand: func(s scenario.SourceSpec) Fetcher { return commandFetcher{command: s.Command} },
	scenario.SourceFile:    func(s scenario.SourceSpec) Fetcher { return fileFetcher{path: s.Path} },
}

// For returns the fetcher of a source.
func (FetcherManager) For(s scenario.SourceSpec) (Fetcher, error) {
	f, ok := fetcherFactories[s.Kind]
	if !ok {
		return nil, fmt.Errorf("unknown source kind %q (http|command|file)", s.Kind)
	}
	return f(s), nil
}

// ProcReader reads /proc files of a pid locally, or through a command prefix (ssh host cat).
type ProcReader struct {
	Prefix []string
}

// Read returns the content of /proc/<pid>/<name>.
func (r ProcReader) Read(ctx context.Context, pid int, name string) ([]byte, error) {
	path := fmt.Sprintf("/proc/%d/%s", pid, name)
	if len(r.Prefix) == 0 {
		return os.ReadFile(path)
	}
	ctx, cancel := context.WithTimeout(ctx, Timeouts.Fetch)
	defer cancel()
	args := append(append([]string{}, r.Prefix[1:]...), path)
	return exec.CommandContext(ctx, r.Prefix[0], args...).Output()
}

// PIDResolver turns fetched bytes into a pid (first integer token).
type PIDResolver struct{}

// Parse returns the pid in raw.
func (PIDResolver) Parse(raw []byte) (int, error) {
	var pid int
	fields := strings.Fields(string(raw))
	if len(fields) == 0 {
		return 0, fmt.Errorf("no pid")
	}
	_, err := fmt.Sscanf(fields[0], "%d", &pid)
	return pid, err
}

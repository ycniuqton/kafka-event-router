// Package runner executes stress scenarios: it provisions stress- topics and routes through the router API, drives
// loadgen processes, samples the system every interval, detects the knee and writes results.
package runner

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/eventrouter/router/test/stress/kit/scenario"
	"github.com/eventrouter/router/test/stress/kit/stream"
)

// LaunchLimits bound loadgen process handling.
var LaunchLimits = struct {
	LineBuffer int
	StopGrace  time.Duration
	Ready      time.Duration
}{LineBuffer: 4 * 1024 * 1024, StopGrace: 2 * time.Minute, Ready: time.Minute}

// Loadgen is one running loadgen subprocess whose JSON-lines report is delivered on Lines.
type Loadgen struct {
	Name  string
	Lines chan stream.Line
	cmd   *exec.Cmd
	stdin io.WriteCloser
	done  chan error
	log   *os.File
}

// Launcher starts loadgen subcommands for a target (command prefix: taskset locally, ssh for a remote host).
type Launcher struct {
	Spec   scenario.LoadgenSpec
	LogDir string
}

// Start runs `loadgen <sub> args... --stop-on-eof`; stderr goes to <LogDir>/<name>.log.
func (l Launcher) Start(ctx context.Context, name, sub string, args []string) (*Loadgen, error) {
	argv := append(append(append([]string{}, l.Spec.Launch...), l.Spec.Binary, sub), args...)
	argv = append(argv, "--stop-on-eof")
	if err := os.MkdirAll(l.LogDir, 0o755); err != nil {
		return nil, err
	}
	logf, err := os.Create(fmt.Sprintf("%s/%s.log", l.LogDir, name))
	if err != nil {
		return nil, err
	}
	fmt.Fprintf(logf, "# %s\n", strings.Join(argv, " "))
	cmd := exec.Command(argv[0], argv[1:]...)
	cmd.Stderr = logf
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		logf.Close()
		return nil, fmt.Errorf("start %s: %w", name, err)
	}
	g := &Loadgen{Name: name, Lines: make(chan stream.Line, 256), cmd: cmd, stdin: stdin, done: make(chan error, 1), log: logf}
	go g.pump(stdout)
	return g, nil
}

// Stop asks the process to finish (stdin EOF also reaches a remote process through ssh).
func (g *Loadgen) Stop() {
	_ = g.stdin.Close()
}

// Wait waits for exit, killing the process after grace.
func (g *Loadgen) Wait(grace time.Duration) error {
	select {
	case err := <-g.done:
		return err
	case <-time.After(grace):
		_ = g.cmd.Process.Kill()
		return fmt.Errorf("%s killed after %s", g.Name, grace)
	}
}

func (g *Loadgen) pump(stdout io.Reader) {
	sc := bufio.NewScanner(stdout)
	sc.Buffer(make([]byte, 64*1024), LaunchLimits.LineBuffer)
	for sc.Scan() {
		var line stream.Line
		if err := json.Unmarshal(sc.Bytes(), &line); err != nil {
			fmt.Fprintf(g.log, "# unparsed stdout: %s\n", sc.Text())
			continue
		}
		g.Lines <- line
	}
	close(g.Lines)
	err := g.cmd.Wait()
	g.log.Close()
	g.done <- err
}

// AwaitReady consumes lines until the measurer reports ready.
func (g *Loadgen) AwaitReady(ctx context.Context) error {
	timeout := time.After(LaunchLimits.Ready)
	for {
		select {
		case line, ok := <-g.Lines:
			if !ok {
				return fmt.Errorf("%s exited before ready (see its log)", g.Name)
			}
			if line.Kind == stream.KindReady {
				return nil
			}
		case <-timeout:
			return fmt.Errorf("%s not ready after %s", g.Name, LaunchLimits.Ready)
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

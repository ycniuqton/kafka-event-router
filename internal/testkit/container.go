package testkit

import (
	"context"
	"fmt"
	"net"
	"os/exec"
	"strings"
	"time"
)

// DockerCommand is the CLI the container provider drives (no Docker SDK dependency).
type DockerCommand string

const DockerBinary DockerCommand = "docker"

// ContainerTiming bounds how long a started container may take to accept TCP connections.
type ContainerTiming time.Duration

const (
	ContainerStartTimeout ContainerTiming = ContainerTiming(90 * time.Second)
	ContainerPollInterval ContainerTiming = ContainerTiming(250 * time.Millisecond)
)

// HostAddress is the interface published container ports bind to.
type HostAddress string

const LoopbackHost HostAddress = "127.0.0.1"

// ContainerProvider starts one throwaway container per requested service through the docker CLI
// and removes it on Release; it reports ErrNotConfigured when docker is not installed.
type ContainerProvider struct {
	catalog ContainerCatalog
}

// NewContainerProvider builds the provider over the given image catalogue.
func NewContainerProvider(catalog ContainerCatalog) *ContainerProvider {
	return &ContainerProvider{catalog: catalog}
}

// Kind implements Provider.
func (p *ContainerProvider) Kind() ProviderKind {
	return ProviderContainer
}

// Provide runs the service image on a free loopback port and waits until it accepts connections.
func (p *ContainerProvider) Provide(ctx context.Context, svc Service) (Endpoint, Release, error) {
	spec, ok := p.catalog[svc]
	if _, err := exec.LookPath(string(DockerBinary)); err != nil || !ok {
		return Endpoint{}, nil, fmt.Errorf("%w: docker unavailable or no image for %s", ErrNotConfigured, svc)
	}
	hostPort, err := p.freeHostPort()
	if err != nil {
		return Endpoint{}, nil, err
	}
	env, url := spec.Render(hostPort)
	id, err := p.run(ctx, spec, hostPort, env)
	if err != nil {
		return Endpoint{}, nil, err
	}
	release := func() { _ = exec.Command(string(DockerBinary), "rm", "-f", id).Run() }
	if err := p.awaitTCP(ctx, hostPort); err != nil {
		release()
		return Endpoint{}, nil, err
	}
	return Endpoint{Service: svc, URL: url, Token: spec.Token}, release, nil
}

func (p *ContainerProvider) run(ctx context.Context, spec ContainerSpec, hostPort string, env []string) (string, error) {
	_, port, _ := net.SplitHostPort(hostPort)
	args := []string{"run", "-d", "--rm", "-p", string(LoopbackHost) + ":" + port + ":" + spec.Port}
	for _, e := range env {
		args = append(args, "-e", e)
	}
	out, err := exec.CommandContext(ctx, string(DockerBinary), append(args, spec.Image)...).Output()
	if err != nil {
		return "", fmt.Errorf("testkit: docker run %s: %w", spec.Image, err)
	}
	return strings.TrimSpace(string(out)), nil
}

func (p *ContainerProvider) freeHostPort() (string, error) {
	l, err := net.Listen("tcp", net.JoinHostPort(string(LoopbackHost), "0"))
	if err != nil {
		return "", fmt.Errorf("testkit: free port: %w", err)
	}
	defer func() { _ = l.Close() }()
	return l.Addr().String(), nil
}

func (p *ContainerProvider) awaitTCP(ctx context.Context, hostPort string) error {
	deadline := time.Now().Add(time.Duration(ContainerStartTimeout))
	for time.Now().Before(deadline) {
		conn, err := (&net.Dialer{}).DialContext(ctx, "tcp", hostPort)
		if err == nil {
			return conn.Close()
		}
		time.Sleep(time.Duration(ContainerPollInterval))
	}
	return fmt.Errorf("testkit: %s not reachable within %s", hostPort, time.Duration(ContainerStartTimeout))
}

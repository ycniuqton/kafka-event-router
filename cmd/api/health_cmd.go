package main

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/eventrouter/router/internal/platform/app"
	"github.com/eventrouter/router/internal/platform/config"
)

// SubcommandHealth probes a readiness URL from inside the container (no shell or TLS-capable wget needed).
const SubcommandHealth Subcommand = "health"

// HealthCLI names fixed texts and limits of `api health`.
type HealthCLI string

const healthUsage HealthCLI = "usage: api health <url>   (https URLs are verified against ROUTER_TLS_CLIENT_CA_FILE when set)"

// HealthTimeout bounds one probe.
type HealthTimeout time.Duration

const DefaultHealthTimeout HealthTimeout = HealthTimeout(3 * time.Second)

// HealthCommand implements `api health <url>`: exit 0 on HTTP 200, 1 otherwise.
type HealthCommand struct {
	out    io.Writer
	lookup config.LookupFunc
}

// NewHealthCommand builds the command writing to out.
func NewHealthCommand(out io.Writer) *HealthCommand {
	return &HealthCommand{out: out, lookup: os.LookupEnv}
}

// Run probes args[0].
func (c *HealthCommand) Run(args []string) app.ExitCode {
	if len(args) != 1 {
		fmt.Fprintln(c.out, healthUsage)
		return app.ExitFailure
	}
	client, err := c.client()
	if err != nil {
		fmt.Fprintln(c.out, err)
		return app.ExitFailure
	}
	resp, err := client.Get(args[0])
	if err != nil {
		fmt.Fprintln(c.out, err)
		return app.ExitFailure
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		fmt.Fprintln(c.out, resp.Status)
		return app.ExitFailure
	}
	return app.ExitOK
}

// client trusts the internal CA (it signed the api certificate) when one is configured.
func (c *HealthCommand) client() (*http.Client, error) {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	if caFile, ok := c.lookup(string(config.EnvTLSClientCAFile)); ok && caFile != "" {
		pem, err := os.ReadFile(caFile)
		if err != nil {
			return nil, err
		}
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM(pem) {
			return nil, fmt.Errorf("health: no certificate in %s", caFile)
		}
		transport.TLSClientConfig = &tls.Config{RootCAs: pool, MinVersion: tls.VersionTLS12}
	}
	return &http.Client{Transport: transport, Timeout: time.Duration(DefaultHealthTimeout)}, nil
}

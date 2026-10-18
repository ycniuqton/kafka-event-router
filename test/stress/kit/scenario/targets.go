package scenario

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"go.yaml.in/yaml/v3"
)

// Placeholder syntax in targets.yaml: ${secret:KEY} (one key of the secrets file) and ${env:NAME}.
var placeholder = regexp.MustCompile(`\$\{(secret|env):([A-Za-z0-9_]+)\}`)

// SecretsFile reads single KEY=value lines without evaluating the file (it is never sourced, values never printed).
type SecretsFile struct {
	Path string
}

// DefaultPath is the staging credentials file.
func (SecretsFile) DefaultPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".secrets", "event-router-staging.env")
}

// Get returns the value of key ("" when the file or key is missing).
func (s SecretsFile) Get(key string) string {
	f, err := os.Open(s.Path)
	if err != nil {
		return ""
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		if v, ok := strings.CutPrefix(sc.Text(), key+"="); ok {
			return strings.Trim(strings.TrimSpace(v), `"'`)
		}
	}
	return ""
}

// TargetLoader reads targets.yaml.
type TargetLoader struct{}

// Load returns the named target with placeholders resolved.
func (l TargetLoader) Load(path, name string) (Target, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return Target{}, err
	}
	all := map[string]yaml.Node{}
	if err := yaml.Unmarshal(raw, &all); err != nil {
		return Target{}, err
	}
	node, ok := all[name]
	if !ok {
		return Target{}, fmt.Errorf("target %q not in %s", name, path)
	}
	var t Target
	if err := node.Decode(&t); err != nil {
		return Target{}, fmt.Errorf("target %q: %w", name, err)
	}
	t.Name = name
	secrets := SecretsFile{Path: t.Secrets}
	if secrets.Path == "" {
		secrets.Path = secrets.DefaultPath()
	}
	return t, Expander{secrets: secrets}.Target(&t)
}

// Expander resolves ${secret:..} / ${env:..} placeholders in every string of a target.
type Expander struct {
	secrets SecretsFile
}

// Target expands t in place; a placeholder resolving to "" is an error naming the key (never the value).
func (e Expander) Target(t *Target) error {
	var missing []string
	expand := func(s *string) { *s = e.expand(*s, &missing) }
	for _, s := range []*string{&t.API.BaseURL, &t.API.Email, &t.API.Password, &t.API.Token, &t.Kafka.PlatformBootstrap, &t.Kafka.RunnerBootstrap, &t.Kafka.LoadgenBootstrap, &t.Loadgen.Binary, &t.Loadgen.OutDir} {
		expand(s)
	}
	for i := range t.Loadgen.Launch {
		expand(&t.Loadgen.Launch[i])
	}
	for i := range t.Metrics {
		expand(&t.Metrics[i].Source.URL)
		expand(&t.Metrics[i].Source.Command)
	}
	for i := range t.Processes {
		expand(&t.Processes[i].PID.Command)
		expand(&t.Processes[i].PID.Path)
		for j := range t.Processes[i].Reader {
			expand(&t.Processes[i].Reader[j])
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("target %s: unresolved placeholders %v", t.Name, missing)
	}
	return nil
}

func (e Expander) expand(s string, missing *[]string) string {
	return placeholder.ReplaceAllStringFunc(s, func(m string) string {
		parts := placeholder.FindStringSubmatch(m)
		v := os.Getenv(parts[2])
		if parts[1] == "secret" {
			v = e.secrets.Get(parts[2])
		}
		if v == "" {
			*missing = append(*missing, parts[1]+":"+parts[2])
		}
		return v
	})
}

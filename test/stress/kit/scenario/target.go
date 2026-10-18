package scenario

// LoginMethod names how the runner signs in to the router API.
type LoginMethod string

const (
	LoginDev   LoginMethod = "dev"
	LoginDex   LoginMethod = "dex"
	LoginToken LoginMethod = "token"
)

// SourceKind names how a sampler obtains bytes (metrics text, a pid, a /proc file).
type SourceKind string

const (
	SourceHTTP    SourceKind = "http"
	SourceCommand SourceKind = "command"
	SourceFile    SourceKind = "file"
)

// Target is one environment the scenarios can run against.
type Target struct {
	Name      string        `yaml:"-"`
	API       APISpec       `yaml:"api"`
	Kafka     KafkaSpec     `yaml:"kafka"`
	Loadgen   LoadgenSpec   `yaml:"loadgen"`
	Metrics   []MetricsSpec `yaml:"metrics"`
	Processes []ProcessSpec `yaml:"processes"`
	EnvTag    string        `yaml:"envTag"`
	Secrets   string        `yaml:"secretsFile"`
}

// APISpec is the router API and its login.
type APISpec struct {
	BaseURL  string      `yaml:"baseURL"`
	Login    LoginMethod `yaml:"login"`
	Email    string      `yaml:"email"`
	Password string      `yaml:"password"`
	Token    string      `yaml:"token"`
}

// KafkaSpec holds the cluster's address as seen by the platform (stored in the connection), by the runner (admin,
// lag) and by loadgen (which may run on another host). TopicConfig is applied to every topic the runner creates
// (e.g. bounded retention on a small shared broker disk).
type KafkaSpec struct {
	PlatformBootstrap string            `yaml:"platformBootstrap"`
	RunnerBootstrap   string            `yaml:"runnerBootstrap"`
	LoadgenBootstrap  string            `yaml:"loadgenBootstrap"`
	TopicConfig       map[string]string `yaml:"topicConfig"`
}

// LoadgenSpec says how loadgen processes are launched: Launch is a command prefix (taskset, ssh ...).
type LoadgenSpec struct {
	Binary string   `yaml:"binary"`
	Launch []string `yaml:"launch"`
	OutDir string   `yaml:"outDir"`
}

// SourceSpec locates bytes to sample.
type SourceSpec struct {
	Kind    SourceKind `yaml:"kind"`
	URL     string     `yaml:"url"`
	Command string     `yaml:"command"`
	Path    string     `yaml:"path"`
}

// MetricsSpec is one Prometheus text endpoint (a worker).
type MetricsSpec struct {
	Name   string     `yaml:"name"`
	Source SourceSpec `yaml:"source"`
}

// ProcessSpec is one process whose /proc stats are sampled; Reader prefixes "cat /proc/<pid>/..." for remote hosts.
type ProcessSpec struct {
	Name   string     `yaml:"name"`
	PID    SourceSpec `yaml:"pid"`
	Reader []string   `yaml:"reader"`
	CPUs   string     `yaml:"cpus"`
}

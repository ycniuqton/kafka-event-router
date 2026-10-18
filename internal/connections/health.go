package connections

import "github.com/eventrouter/router/pkg/kafka"

// HealthMapper maps the first failed step of a run to the connection health.
type HealthMapper struct {
	byStep   map[TestStep]HealthStatus
	fallback HealthStatus
}

// DefaultHealthMapper is the design §6 table.
func DefaultHealthMapper() HealthMapper {
	return HealthMapper{
		byStep:   map[TestStep]HealthStatus{StepBrokersReachable: HealthUnreachable, StepAuthenticated: HealthAuthFailed},
		fallback: HealthDegraded,
	}
}

// Map returns healthy without a failure, else the status of the failed step.
func (m HealthMapper) Map(failure *LastFailure) HealthStatus {
	if failure == nil {
		return HealthHealthy
	}
	if status, ok := m.byStep[failure.Step]; ok {
		return status
	}
	return m.fallback
}

// CheckPlan is the ordered list of steps a run executes.
type CheckPlan []TestStep

// DefaultTestPlan is the MVP checklist of user-triggered tests.
var DefaultTestPlan = CheckPlan{StepBrokersReachable, StepAuthenticated, StepListTopics, StepCanRead, StepCanWrite}

// DefaultProbePlan is the background health probe (no read/write).
var DefaultProbePlan = CheckPlan{StepBrokersReachable, StepAuthenticated, StepListTopics}

// StepCondition decides whether an optional step joins a plan for a connection spec.
type StepCondition interface {
	Applies(spec kafka.ConnectionSpec) bool
}

// SchemaRegistryConfigured applies when the connection has a schema registry.
type SchemaRegistryConfigured struct{}

// Applies implements StepCondition.
func (SchemaRegistryConfigured) Applies(spec kafka.ConnectionSpec) bool {
	return spec.SchemaRegistry != nil
}

// PlanExtension appends Step to a plan when Condition applies.
type PlanExtension struct {
	Step      TestStep
	Condition StepCondition
}

// PlanExtensions are the optional steps of every plan (tests and probes).
type PlanExtensions []PlanExtension

// DefaultPlanExtensions adds the schema registry check to connections that have one.
var DefaultPlanExtensions = PlanExtensions{{Step: StepSchemaRegistry, Condition: SchemaRegistryConfigured{}}}

// Apply returns plan plus every applicable step it does not already contain.
func (e PlanExtensions) Apply(plan CheckPlan, spec kafka.ConnectionSpec) CheckPlan {
	out := append(CheckPlan{}, plan...)
	for _, ext := range e {
		if ext.Condition.Applies(spec) && !out.Has(ext.Step) {
			out = append(out, ext.Step)
		}
	}
	return out
}

// Has reports whether the plan contains step.
func (p CheckPlan) Has(step TestStep) bool {
	for _, s := range p {
		if s == step {
			return true
		}
	}
	return false
}

// Hint is the plain-language explanation of a failure: what happened, why, what to do.
type Hint struct {
	Message string
	Hint    string
}

// HintKey selects a hint; an empty Step is the reason's default.
type HintKey struct {
	Step   TestStep
	Reason kafka.FailureReason
}

// HintCatalog is the domain table of failure explanations.
type HintCatalog struct {
	entries map[HintKey]Hint
}

// Lookup returns the step-specific hint, the reason default, or the unknown hint.
func (c *HintCatalog) Lookup(step TestStep, reason kafka.FailureReason) Hint {
	if h, ok := c.entries[HintKey{Step: step, Reason: reason}]; ok {
		return h
	}
	if h, ok := c.entries[HintKey{Reason: reason}]; ok {
		return h
	}
	return c.entries[HintKey{Reason: kafka.ReasonUnknown}]
}

// DefaultHintCatalog holds the UI §12 texts.
func DefaultHintCatalog() *HintCatalog {
	return &HintCatalog{entries: map[HintKey]Hint{
		{Reason: kafka.ReasonUnreachable}:                             {"Can't reach any broker.", "The servers may be down or blocked by a firewall. Check the bootstrap servers and network rules, then test again."},
		{Reason: kafka.ReasonDNSFailure}:                              {"Broker host name not found.", "Check the spelling of the bootstrap servers and that DNS resolves them from the router's network."},
		{Reason: kafka.ReasonTLSHandshake}:                            {"Secure connection failed.", "The broker and this connection disagree on TLS. Turn TLS on or off to match the broker listener, then test again."},
		{Reason: kafka.ReasonTLSUntrusted}:                            {"The broker's certificate is not trusted.", "Add the CA certificate (.pem) that signed the broker certificate, or check the TLS server name."},
		{Reason: kafka.ReasonAuthFailed}:                              {"Authentication failed.", "Check the username and password, and that the user exists on the cluster with this SASL mechanism."},
		{Reason: kafka.ReasonSASLMismatch}:                            {"Authentication failed.", "Check the SASL mechanism: the broker does not accept the selected one."},
		{Reason: kafka.ReasonNotAuthorized}:                           {"Access denied.", "The user is authenticated but lacks permission for this operation. Ask the cluster admin to grant it (ACLs)."},
		{Reason: kafka.ReasonTopicNotFound}:                           {"Topic not found.", "Pick an existing topic, or create it first."},
		{Reason: kafka.ReasonTopicExists}:                             {"Topic already exists.", "Pick another name or use the existing topic."},
		{Reason: kafka.ReasonInvalidConfig}:                           {"The cluster rejected the request.", "Check partitions, replication factor and topic settings against the cluster limits."},
		{Reason: kafka.ReasonTimeout}:                                 {"The cluster did not answer in time.", "The broker may be overloaded or unreachable. Test again; if it persists, check the network path."},
		{Reason: kafka.ReasonUnknown}:                                 {"The check failed.", "Test again. If it keeps failing, check the broker logs or contact the cluster admin."},
		{Step: StepListTopics, Reason: kafka.ReasonNotAuthorized}:     {"Can't list topics.", "Grant DESCRIBE on topics to this user, then test again."},
		{Step: StepCanRead, Reason: kafka.ReasonNotAuthorized}:        {"Can't read from the sample topic.", "Grant READ on this topic to this user, then test again."},
		{Step: StepCanWrite, Reason: kafka.ReasonNotAuthorized}:       {"Can't write to the test topic.", "Grant WRITE on this topic to this user, or pick another test topic."},
		{Step: StepSchemaRegistry, Reason: kafka.ReasonUnreachable}:   {"Can't reach the schema registry.", "Check the registry URL and that the router's network can reach it, then test again."},
		{Step: StepSchemaRegistry, Reason: kafka.ReasonDNSFailure}:    {"Schema registry host name not found.", "Check the spelling of the registry URL and that DNS resolves it from the router's network."},
		{Step: StepSchemaRegistry, Reason: kafka.ReasonAuthFailed}:    {"The schema registry rejected the credentials.", "Check the registry authentication mode, username and password or token."},
		{Step: StepSchemaRegistry, Reason: kafka.ReasonNotAuthorized}: {"The schema registry denied access.", "Grant this user read access to subjects in the registry, then test again."},
		{Step: StepSchemaRegistry, Reason: kafka.ReasonInvalidConfig}: {"No schema registry answers at this URL.", "Point the URL at the registry root, for example https://schema-registry.internal:8081."},
		{Step: StepSchemaRegistry, Reason: kafka.ReasonUnknown}:       {"The schema registry check failed.", "Test again. If it keeps failing, check the registry logs or contact its admin."},
	}}
}

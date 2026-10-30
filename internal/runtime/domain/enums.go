// Package domain holds the route-runtime vocabulary shared by workers, the internal worker API and
// the scheduler: enums, timing constants and the worker ↔ control-plane wire contract.
package domain

// RunnerState is the lifecycle state of one route runner on one worker.
type RunnerState string

const (
	RunnerStarting RunnerState = "starting"
	RunnerRunning  RunnerState = "running"
	RunnerDraining RunnerState = "draining"
	RunnerPaused   RunnerState = "paused"
	RunnerError    RunnerState = "error"
	RunnerStopped  RunnerState = "stopped"
)

// ObservedState mirrors the route_observed_state Postgres enum.
type ObservedState string

const (
	ObservedRunning ObservedState = "running"
	ObservedPaused  ObservedState = "paused"
	ObservedError   ObservedState = "error"
)

// observedByRunner folds runner states into the observed state shown to users.
var observedByRunner = map[RunnerState]ObservedState{
	RunnerStarting: ObservedRunning,
	RunnerRunning:  ObservedRunning,
	RunnerDraining: ObservedRunning,
	RunnerPaused:   ObservedPaused,
	RunnerStopped:  ObservedPaused,
	RunnerError:    ObservedError,
}

// Observed maps the runner state to route_observed_state (unknown states read as error).
func (s RunnerState) Observed() ObservedState {
	if o, ok := observedByRunner[s]; ok {
		return o
	}
	return ObservedError
}

// ErrorCategory classifies every runtime failure (design §2.5).
type ErrorCategory string

const (
	CategoryNone                 ErrorCategory = ""
	CategoryDecode               ErrorCategory = "decode"
	CategoryStep                 ErrorCategory = "step"
	CategoryStepTimeout          ErrorCategory = "step_timeout"
	CategoryOutputInvalid        ErrorCategory = "output_invalid"
	CategoryDestinationTransient ErrorCategory = "destination_transient"
	CategoryDestinationPermanent ErrorCategory = "destination_permanent"
	CategorySourceTransient      ErrorCategory = "source_transient"
	CategorySourceFatal          ErrorCategory = "source_fatal"
	CategoryCompile              ErrorCategory = "compile"
	CategoryDependencyTransient  ErrorCategory = "dependency_transient"
)

// Disposition is what the runner does with a classified failure.
type Disposition string

const (
	DispositionRetryInMemory Disposition = "retry_in_memory"
	DispositionApplyPolicy   Disposition = "apply_policy"
	DispositionBackoffRetry  Disposition = "backoff_retry"
	DispositionHaltRoute     Disposition = "halt_route"
)

// ReloadKind is how a runner moves from one version to the next.
type ReloadKind string

const (
	ReloadNone         ReloadKind = "none"
	ReloadPipelineSwap ReloadKind = "pipeline_swap"
	ReloadFullRestart  ReloadKind = "full_restart"
)

// RecordOutcome is how one source record finished.
type RecordOutcome string

const (
	OutcomeProduced       RecordOutcome = "produced"
	OutcomeFiltered       RecordOutcome = "filtered"
	OutcomeDropped        RecordOutcome = "dropped"
	OutcomeDeadLettered   RecordOutcome = "dead_lettered"
	OutcomeSkipped        RecordOutcome = "skipped"
	OutcomeRetryScheduled RecordOutcome = "retry_scheduled"
)

// HaltReason explains why a route was halted without restart loop.
type HaltReason string

const (
	HaltErrorPolicyPause            HaltReason = "error_policy_pause"
	HaltDestinationPermanent        HaltReason = "destination_permanent"
	HaltSourceFatal                 HaltReason = "source_fatal"
	HaltAutoRollbackFailed          HaltReason = "auto_rollback_failed"
	HaltDLQTopicUnavailable         HaltReason = "dlq_topic_unavailable"
	HaltDestinationTopicUnavailable HaltReason = "destination_topic_unavailable"
	HaltDeliveryModeForbidden       HaltReason = "delivery_mode_forbidden"
	HaltTransactionFailed           HaltReason = "transaction_failed"
)

// recoverableHalts are runner-local failures fixed by reopening the runner's clients, not by the user.
var recoverableHalts = map[HaltReason]bool{HaltTransactionFailed: true}

// Recoverable reports whether the worker restarts the runner itself instead of halting the route.
func (r HaltReason) Recoverable() bool { return recoverableHalts[r] }

// StopReason explains why a runner was stopped.
type StopReason string

const (
	StopUnassigned StopReason = "unassigned"
	StopHalt       StopReason = "halt"
	StopRestart    StopReason = "restart"
	StopShutdown   StopReason = "shutdown"
)

// ChangeKind names what a change hint is about (NOTIFY payload kind = source table).
type ChangeKind string

const (
	ChangeRoute      ChangeKind = "route"
	ChangeAssignment ChangeKind = "assignment"
	ChangeDeployment ChangeKind = "deployment"
	ChangeRuntime    ChangeKind = "runtime"
	ChangeReplay     ChangeKind = "replay"
	ChangeResync     ChangeKind = "resync"
)

// NotifyChannel is the Postgres channel the 0008 trigger publishes on.
type NotifyChannel string

const ChannelRouterRuntime NotifyChannel = "router_runtime"

// DLQTargetKind selects the default DLQ connection when a route sets none (ROUTER_DLQ_DEFAULT_TARGET).
type DLQTargetKind string

const (
	DLQTargetSource      DLQTargetKind = "source"
	DLQTargetDestination DLQTargetKind = "destination"
)

// ProducerSharing toggles whether routes share producer clients (ROUTER_WORKER_PRODUCER_SHARING, on since P2).
type ProducerSharing string

const (
	SharingOff ProducerSharing = "off"
	SharingOn  ProducerSharing = "on"
)

// Valid reports whether s is a known sharing mode.
func (s ProducerSharing) Valid() bool {
	return s == SharingOff || s == SharingOn
}

// PlacementKind names a scheduler placement strategy (ROUTER_SCHEDULER_PLACEMENT).
type PlacementKind string

const (
	PlacementLeastLoaded  PlacementKind = "least_loaded"
	PlacementSingleWorker PlacementKind = "single_worker"
)

// ReplayStatus mirrors the replay_status Postgres enum (migration 2060).
type ReplayStatus string

const (
	ReplayPending   ReplayStatus = "pending"
	ReplayStopping  ReplayStatus = "stopping"
	ReplayResetting ReplayStatus = "resetting"
	ReplayDone      ReplayStatus = "done"
	ReplayFailed    ReplayStatus = "failed"
)

// suspendingReplays are the statuses during which the route's runners stay suspended.
var suspendingReplays = map[ReplayStatus]bool{ReplayStopping: true, ReplayResetting: true}

// Suspends reports whether a replay in this status keeps the route's runners stopped.
func (s ReplayStatus) Suspends() bool { return suspendingReplays[s] }

// Open reports whether the replay is still in progress.
func (s ReplayStatus) Open() bool { return s == ReplayPending || s.Suspends() }

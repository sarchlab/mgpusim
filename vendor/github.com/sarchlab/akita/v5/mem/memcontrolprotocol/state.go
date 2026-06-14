package memcontrolprotocol

// State enumerates the control lifecycle states every memory agent moves
// through. Reset and Invalidate are operations within these states, not
// states themselves; they leave the resulting state unchanged (except
// that Reset always lands the component back in Enabled).
type State int

const (
	// StateEnabled is the normal operating state. The component accepts
	// new traffic and schedules internal work.
	StateEnabled State = iota

	// StatePausing is the transient state between receiving a Pause
	// request and finishing the current tick. Most components collapse
	// this into StatePaused within the same tick.
	StatePausing

	// StatePaused means no new traffic is accepted and no internal
	// progress is made. In-flight transactions are frozen in place.
	StatePaused

	// StateDraining means a Drain request is in flight: no new traffic
	// is accepted, but in-flight transactions are allowed to finish.
	// The component transitions to StatePaused when quiescent.
	StateDraining

	// StateFlushing means a Flush request is in flight: dirty private
	// state is being written back to the backing memory. The component
	// transitions back to its prior paused/drained state when done.
	StateFlushing
)

// String returns a stable human-readable name for the state, suitable
// for tracing and error messages.
func (s State) String() string {
	switch s {
	case StateEnabled:
		return "enabled"
	case StatePausing:
		return "pausing"
	case StatePaused:
		return "paused"
	case StateDraining:
		return "draining"
	case StateFlushing:
		return "flushing"
	default:
		return "unknown"
	}
}

// VerbSupport declares which control verbs a component implements.
// Verbs left false must respond with ControlRsp{Success: false,
// Error: ErrUnsupported}.
type VerbSupport struct {
	Pause      bool
	Drain      bool
	Enable     bool
	Reset      bool
	Invalidate bool
	Flush      bool
}

// Supports reports whether the given command is in the support set.
func (v VerbSupport) Supports(cmd Command) bool {
	switch cmd {
	case CmdPause:
		return v.Pause
	case CmdDrain:
		return v.Drain
	case CmdEnable:
		return v.Enable
	case CmdReset:
		return v.Reset
	case CmdInvalidate:
		return v.Invalidate
	case CmdFlush:
		return v.Flush
	default:
		return false
	}
}

// Universal is the matrix every memory agent must satisfy: the four
// universal verbs (Pause, Drain, Enable, Reset). The two conditional
// verbs (Invalidate, Flush) are off.
func Universal() VerbSupport {
	return VerbSupport{
		Pause:  true,
		Drain:  true,
		Enable: true,
		Reset:  true,
	}
}

// CacheLike is the support matrix for components that hold dirty
// private cache-of-memory state (write-back style caches): all four
// universal verbs plus Invalidate and Flush.
func CacheLike() VerbSupport {
	v := Universal()
	v.Invalidate = true
	v.Flush = true
	return v
}

// TranslationCacheLike is the support matrix for components that hold
// a private cache of translations (TLB, MMU cache): the four universal
// verbs plus Invalidate. Flush is not meaningful because translations
// are never dirty.
func TranslationCacheLike() VerbSupport {
	v := Universal()
	v.Invalidate = true
	return v
}

// Protocol-defined Error strings on ControlRsp.
const (
	// ErrUnsupported means the component does not implement the verb.
	ErrUnsupported = "unsupported"

	// ErrMustBePausedOrDrained means Invalidate or Flush was issued
	// while the component was Enabled. The caller must Pause or Drain
	// first.
	ErrMustBePausedOrDrained = "must be paused or drained"
)

// IsSyncVerb reports whether the verb is acknowledged synchronously
// (Pause, Enable, Reset, Invalidate) versus asynchronously on
// completion (Drain, Flush). Used by the contract harness to know
// whether to expect a Rsp within one tick or to keep ticking.
func IsSyncVerb(cmd Command) bool {
	switch cmd {
	case CmdPause, CmdEnable, CmdReset, CmdInvalidate:
		return true
	case CmdDrain, CmdFlush:
		return false
	default:
		return true
	}
}
